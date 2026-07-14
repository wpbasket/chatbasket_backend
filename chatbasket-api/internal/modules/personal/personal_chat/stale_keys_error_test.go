package personal_chat

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"chatbasket-api/internal/platform/kit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newStaleKeysError wraps kit.NewErrorWithDetails to keep tests terse and
// preserve the previous message format.
func newStaleKeysError(details StaleKeysErrorDetails) error {
	return kit.NewErrorWithDetails(
		http.StatusConflict,
		"keys_stale",
		fmt.Sprintf("keys_revision is stale (side: %s)", details.StaleSide),
		details,
	)
}

// ============================================================================
// StaleKeysErrorDetails Implementation Tests (using kit.NewErrorWithDetails)
// ============================================================================

func TestStaleKeysError_ImplementsProcessedError(t *testing.T) {
	details := StaleKeysErrorDetails{
		StaleSide:             StaleSideRecipient,
		RecipientKeysRevision: 7,
		RecipientActiveKeys:   []string{"key1", "key2"},
	}

	err := newStaleKeysError(details)

	var pe kit.ProcessedError
	assert.True(t, errors.As(err, &pe), "should implement ProcessedError")
	assert.Equal(t, 409, pe.Status())
	assert.Equal(t, "keys_stale", pe.Kind())
	assert.Contains(t, pe.Error(), "keys_revision is stale")
}

func TestStaleKeysError_ImplementsDetailedProcessedError(t *testing.T) {
	details := StaleKeysErrorDetails{
		StaleSide:             StaleSideRecipient,
		RecipientKeysRevision: 7,
		RecipientActiveKeys:   []string{"key1", "key2"},
	}

	err := newStaleKeysError(details)

	var dpe kit.DetailedProcessedError
	assert.True(t, errors.As(err, &dpe), "should implement DetailedProcessedError")

	returnedDetails := dpe.Details()
	assert.NotNil(t, returnedDetails)

	returnedStaleDetails, ok := returnedDetails.(StaleKeysErrorDetails)
	require.True(t, ok, "Details should be StaleKeysErrorDetails")
	assert.Equal(t, StaleSideRecipient, returnedStaleDetails.StaleSide)
	assert.Equal(t, int32(7), returnedStaleDetails.RecipientKeysRevision)
	assert.Equal(t, []string{"key1", "key2"}, returnedStaleDetails.RecipientActiveKeys)
}

// ============================================================================
// HTTP Error Response Tests
// ============================================================================

func TestStaleKeysError_HTTPResponse_IncludesDetails(t *testing.T) {
	details := StaleKeysErrorDetails{
		StaleSide:             StaleSideRecipient,
		RecipientKeysRevision: 7,
		RecipientActiveKeys:   []string{"device1_key_base64", "device2_key_base64"},
	}

	err := newStaleKeysError(details)

	var pe kit.ProcessedError
	require.True(t, errors.As(err, &pe))

	apiErr := kit.ApiError{
		Code:    pe.Status(),
		Type:    pe.Kind(),
		Message: pe.Error(),
	}

	if dpe, ok := pe.(kit.DetailedProcessedError); ok {
		apiErr.Details = dpe.Details()
	}

	jsonBytes, marshalErr := json.Marshal(apiErr)
	require.NoError(t, marshalErr)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(jsonBytes, &result))

	assert.Equal(t, float64(409), result["code"])
	assert.Equal(t, "keys_stale", result["type"])
	assert.Contains(t, result["message"], "keys_revision is stale")

	detailsMap, ok := result["details"].(map[string]interface{})
	require.True(t, ok, "details field should exist and be a map")
	assert.Equal(t, "recipient", detailsMap["stale_side"])
	assert.Equal(t, float64(7), detailsMap["recipient_keys_revision"])

	activeKeys, ok := detailsMap["recipient_active_keys"].([]interface{})
	require.True(t, ok, "recipient_active_keys should be an array")
	assert.Len(t, activeKeys, 2)
	assert.Equal(t, "device1_key_base64", activeKeys[0])
	assert.Equal(t, "device2_key_base64", activeKeys[1])
}

func TestStaleKeysError_HTTPResponse_RecipientStale(t *testing.T) {
	details := StaleKeysErrorDetails{
		StaleSide:             StaleSideRecipient,
		RecipientKeysRevision: 7,
		RecipientActiveKeys:   []string{"key1"},
	}

	err := newStaleKeysError(details)
	var pe kit.ProcessedError
	require.True(t, errors.As(err, &pe))

	apiErr := kit.ApiError{
		Code:    pe.Status(),
		Type:    pe.Kind(),
		Message: pe.Error(),
	}

	if dpe, ok := pe.(kit.DetailedProcessedError); ok {
		apiErr.Details = dpe.Details()
	}

	jsonBytes, marshalErr := json.Marshal(apiErr)
	require.NoError(t, marshalErr)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(jsonBytes, &result))
	detailsMap := result["details"].(map[string]interface{})

	_, hasSenderRevision := detailsMap["sender_keys_revision"]
	_, hasSenderKeys := detailsMap["sender_active_keys"]
	assert.False(t, hasSenderRevision, "sender_keys_revision should be omitted")
	assert.False(t, hasSenderKeys, "sender_active_keys should be omitted")
}

func TestStaleKeysError_HTTPResponse_SenderStale(t *testing.T) {
	details := StaleKeysErrorDetails{
		StaleSide:          StaleSideSender,
		SenderKeysRevision: 3,
		SenderActiveKeys:   []string{"sender_key1"},
	}

	err := newStaleKeysError(details)
	var pe kit.ProcessedError
	require.True(t, errors.As(err, &pe))

	apiErr := kit.ApiError{
		Code:    pe.Status(),
		Type:    pe.Kind(),
		Message: pe.Error(),
	}

	if dpe, ok := pe.(kit.DetailedProcessedError); ok {
		apiErr.Details = dpe.Details()
	}

	jsonBytes, marshalErr := json.Marshal(apiErr)
	require.NoError(t, marshalErr)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(jsonBytes, &result))
	detailsMap := result["details"].(map[string]interface{})

	assert.Equal(t, "sender", detailsMap["stale_side"])
	assert.Equal(t, float64(3), detailsMap["sender_keys_revision"])

	_, hasRecipientRevision := detailsMap["recipient_keys_revision"]
	_, hasRecipientKeys := detailsMap["recipient_active_keys"]
	assert.False(t, hasRecipientRevision, "recipient_keys_revision should be omitted")
	assert.False(t, hasRecipientKeys, "recipient_active_keys should be omitted")
}

func TestStaleKeysError_HTTPResponse_BothStale(t *testing.T) {
	details := StaleKeysErrorDetails{
		StaleSide:             StaleSideBoth,
		SenderKeysRevision:    3,
		SenderActiveKeys:      []string{"sender_key1"},
		RecipientKeysRevision: 7,
		RecipientActiveKeys:   []string{"recipient_key1", "recipient_key2"},
	}

	err := newStaleKeysError(details)
	var pe kit.ProcessedError
	require.True(t, errors.As(err, &pe))

	apiErr := kit.ApiError{
		Code:    pe.Status(),
		Type:    pe.Kind(),
		Message: pe.Error(),
	}

	if dpe, ok := pe.(kit.DetailedProcessedError); ok {
		apiErr.Details = dpe.Details()
	}

	jsonBytes, marshalErr := json.Marshal(apiErr)
	require.NoError(t, marshalErr)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(jsonBytes, &result))
	detailsMap := result["details"].(map[string]interface{})

	assert.Equal(t, "both", detailsMap["stale_side"])
	assert.Equal(t, float64(3), detailsMap["sender_keys_revision"])
	assert.Equal(t, float64(7), detailsMap["recipient_keys_revision"])

	senderKeys := detailsMap["sender_active_keys"].([]interface{})
	assert.Len(t, senderKeys, 1)

	recipientKeys := detailsMap["recipient_active_keys"].([]interface{})
	assert.Len(t, recipientKeys, 2)
}

// ============================================================================
// WebSocket Error Response Tests
// ============================================================================

func TestToWSError_IncludesDetails(t *testing.T) {
	details := StaleKeysErrorDetails{
		StaleSide:             StaleSideRecipient,
		RecipientKeysRevision: 7,
		RecipientActiveKeys:   []string{"key1", "key2"},
	}

	err := newStaleKeysError(details)
	wsErr := toWSError(err)

	assert.Equal(t, 409, wsErr.Code)
	assert.Equal(t, "keys_stale", wsErr.Type)
	assert.Contains(t, wsErr.Message, "keys_revision is stale")

	assert.NotNil(t, wsErr.Details, "details should not be nil")

	returnedDetails, ok := wsErr.Details.(StaleKeysErrorDetails)
	require.True(t, ok, "details should be StaleKeysErrorDetails")
	assert.Equal(t, StaleSideRecipient, returnedDetails.StaleSide)
	assert.Equal(t, int32(7), returnedDetails.RecipientKeysRevision)
	assert.Equal(t, []string{"key1", "key2"}, returnedDetails.RecipientActiveKeys)
}

func TestToWSError_RegularError_NoDetails(t *testing.T) {
	err := kit.NewError(404, "not_found", "User not found")
	wsErr := toWSError(err)

	assert.Equal(t, 404, wsErr.Code)
	assert.Equal(t, "not_found", wsErr.Type)
	assert.Equal(t, "User not found", wsErr.Message)
	assert.Nil(t, wsErr.Details, "details should be nil for regular errors")
}

func TestToWSError_JSONSerialization_IncludesDetails(t *testing.T) {
	details := StaleKeysErrorDetails{
		StaleSide:             StaleSideRecipient,
		RecipientKeysRevision: 7,
		RecipientActiveKeys:   []string{"key1"},
	}

	err := newStaleKeysError(details)
	wsErr := toWSError(err)

	jsonBytes, marshalErr := json.Marshal(wsErr)
	require.NoError(t, marshalErr)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(jsonBytes, &result))

	assert.Equal(t, float64(409), result["code"])
	assert.Equal(t, "keys_stale", result["type"])
	assert.Contains(t, result["message"], "keys_revision is stale")

	detailsMap, ok := result["details"].(map[string]interface{})
	require.True(t, ok, "details field should exist and be a map")
	assert.Equal(t, "recipient", detailsMap["stale_side"])
	assert.Equal(t, float64(7), detailsMap["recipient_keys_revision"])
}

func TestToWSError_RegularError_JSONSerialization_OmitsDetails(t *testing.T) {
	err := kit.NewError(400, "bad_request", "Invalid input")
	wsErr := toWSError(err)

	jsonBytes, marshalErr := json.Marshal(wsErr)
	require.NoError(t, marshalErr)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(jsonBytes, &result))

	_, hasDetails := result["details"]
	assert.False(t, hasDetails, "details field should be omitted for regular errors")
}

// ============================================================================
// Backward Compatibility Tests
// ============================================================================

func TestExistingErrors_StillWork_NoDetails(t *testing.T) {
	testCases := []struct {
		name string
		err  error
	}{
		{"NotFoundError", kit.NewError(404, "not_found", "Resource not found")},
		{"BadRequestError", kit.NewError(400, "bad_request", "Invalid input")},
		{"UnauthorizedError", kit.NewError(401, "unauthorized", "Not authorized")},
		{"InternalError", kit.NewError(500, "internal_error", "Something went wrong")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var pe kit.ProcessedError
			require.True(t, errors.As(tc.err, &pe))

			apiErr := kit.ApiError{
				Code:    pe.Status(),
				Type:    pe.Kind(),
				Message: pe.Error(),
			}

			if dpe, ok := pe.(kit.DetailedProcessedError); ok {
				apiErr.Details = dpe.Details()
			}

			jsonBytes, err := json.Marshal(apiErr)
			require.NoError(t, err)

			var result map[string]interface{}
			require.NoError(t, json.Unmarshal(jsonBytes, &result))

			_, hasDetails := result["details"]
			assert.False(t, hasDetails, "existing errors should not have details field")

			wsErr := toWSError(tc.err)
			assert.Nil(t, wsErr.Details, "existing errors should have nil details in WS")
		})
	}
}

// ============================================================================
// Edge Cases
// ============================================================================

func TestStaleKeysError_EmptyKeys(t *testing.T) {
	details := StaleKeysErrorDetails{
		StaleSide:             StaleSideRecipient,
		RecipientKeysRevision: 0,
		RecipientActiveKeys:   []string{},
	}

	err := newStaleKeysError(details)
	wsErr := toWSError(err)

	assert.NotNil(t, wsErr.Details)
	returnedDetails := wsErr.Details.(StaleKeysErrorDetails)
	assert.Equal(t, int32(0), returnedDetails.RecipientKeysRevision)
	assert.Empty(t, returnedDetails.RecipientActiveKeys)
}

func TestStaleKeysError_MultipleKeys(t *testing.T) {
	details := StaleKeysErrorDetails{
		StaleSide:             StaleSideRecipient,
		RecipientKeysRevision: 10,
		RecipientActiveKeys:   []string{"key1", "key2", "key3", "key4", "key5"},
	}

	err := newStaleKeysError(details)
	wsErr := toWSError(err)

	returnedDetails := wsErr.Details.(StaleKeysErrorDetails)
	assert.Len(t, returnedDetails.RecipientActiveKeys, 5)
}
