package services

import (
	"chatbasket/model"
	"chatbasket/utils"
	"context"
	"log"
	"time"

	"github.com/appwrite/sdk-for-go/id"
	"github.com/appwrite/sdk-for-go/query"
	"github.com/google/uuid"
)

func (ps *GlobalService) UpdatePassword(ctx context.Context, payload *model.UpdatePassword, userId string) (*model.StatusOkay, *model.ApiError) {
	// doc, err := ps.Appwrite.Users.Get(userId)
	// if err != nil {
	// 	return nil, echo.NewHTTPError(401, "Failed to query user data: "+err.Error())
	// }

	// match, err := utils.VerifyOTP(payload.OldPassword, doc.Password)
	// if err != nil {
	// 	return nil, echo.NewHTTPError(500, "Failed to verify old password: "+err.Error())
	// }
	// if !match {
	// 	return nil, echo.NewHTTPError(401, "Old password does not match")
	// }
	newPass:="00"+payload.NewPassword
	_, err := ps.Appwrite.Users.UpdatePassword(userId, newPass)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to update password: " + err.Error(),

		}
	}

	return &model.StatusOkay{Status: true, Message: "Password updated successfully"}, nil
}

func (ps *GlobalService) UpdateEmail(ctx context.Context, payload *model.UpdateEmailPayload, userId string) (*model.StatusOkay, *model.ApiError) {

	res, err := ps.Appwrite.Database.ListDocuments(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.UsersCollectionID,
		ps.Appwrite.Database.WithListDocumentsQueries(
			[]string{
				query.Equal("email", payload.Email),
				query.Limit(1),
			},
		),
	)

	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to query user data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	if res.Total == 1 {
		return nil, &model.ApiError{
			Code:    409,
			Message: "Email already registered",
			Type:    "conflict",
		}
	}

	// Create temp email target for sending email

	checkTargets,err:= ps.Appwrite.Users.ListTargets(userId)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to query target data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	if checkTargets.Total == 2{
		_,err:=ps.Appwrite.Users.DeleteTarget(userId,userId)
		if err != nil {
			return nil, &model.ApiError{
				Code:    500,
				Message: "Failed to delete target: " + err.Error(),
				Type:    "internal_server_error",
			}
		}
	}

	_, err = ps.Appwrite.Users.CreateTarget(
		userId,
		userId,
		"email",
		payload.Email,
	)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to create target: " + err.Error(),
			Type:    "internal_server_error",
		}
	}


	messageId := id.Custom(uuid.NewString())
	subject := "Otp for email verification"
	otp, err := utils.GenerateOTP()
	if err != nil {
		return nil,	&model.ApiError{
			Code:    500,
			Message: "Failed to generate OTP: " + err.Error(),
			Type:    "internal_server_error",
		}
		 
	}
	content := "<p>Hello,<br>Please enter this code in the app to verify your email address. This code is valid for 3 minutes.Your One-Time Password (OTP) for verifying your email address is:<br><h1>" + otp + "</h1></p><p>Thank you,<br>ChatBasket</p>"

	_, err = ps.Appwrite.Message.CreateEmail(
		messageId,
		subject,
		content,
		ps.Appwrite.Message.WithCreateEmailTargets([]string{userId}),
	)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to send email: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	docOtp, err := ps.Appwrite.Database.ListDocuments(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.TempOtpCollectionID,
		ps.Appwrite.Database.WithListDocumentsQueries(
			[]string{
				query.Equal("userId", userId),
				query.Limit(1),
			},
		),
	)

	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to query otp data: " + err.Error(),
			Type:    "internal_server_error",
		}
		
	}
	if docOtp.Total == 1 {
		_, err = ps.Appwrite.Database.DeleteDocument(
			ps.Appwrite.DatabaseID,
			ps.Appwrite.TempOtpCollectionID,
			userId,
		)
		if err != nil {
			return nil, &model.ApiError{
				Code:    500,
				Message: "Failed to delete existing otp: " + err.Error(),
				Type:    "internal_server_error",
			}
		}
	}

	hashedOtp, err := utils.HashOTP(otp)
	if err != nil {
		return nil, &model.ApiError{
				Code:    500,
				Message: "Failed to hash OTP: " + err.Error(),
				Type:    "internal_server_error",
			}
			
	}

	tempOtpPayload := model.TempOtpPayload{
		Email:     payload.Email,
		Otp:       hashedOtp,
		UserId:    userId,
		MessageId: messageId,
	}

	_, err = ps.Appwrite.Database.CreateDocument(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.TempOtpCollectionID,
		userId,
		tempOtpPayload,
	)

	if err != nil {
		return nil, &model.ApiError{
				Code:    500,
				Message: "Failed to save otp in database: " + err.Error(),
				Type:    "internal_server_error",
			}
			
	}

	return &model.StatusOkay{Status: true, Message: "Otp sent to new email for verification"}, nil
}

func (ps *GlobalService) UpdateEmailVerification(ctx context.Context, payload *model.UpdateEmailVerification, userId string) (*model.StatusOkay, *model.ApiError) {

	// Retrieve the temporary OTP document from the database
	doc, err := ps.Appwrite.Database.GetDocument(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.TempOtpCollectionID,
		userId,
	)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to query otp data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	var tempOtp model.TempOtp
	if err := doc.Decode(&tempOtp); err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to parse otp data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	if tempOtp.Email != payload.Email {
		return nil, &model.ApiError{
			Code:    401,
			Message: "Email does not match with the sent OTP email",
			Type:    "unauthorized",
		}
	}

	match, err := utils.VerifyOTP(payload.Otp, tempOtp.Otp)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to verify OTP: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	if !match {
		return nil, &model.ApiError{
			Code:    401,
			Message: "Invalid OTP",
			Type:    "unauthorized",
		}
	}

	// check if tempOtp has expired or not time limit is till 3 minutes after created at

	createdAtTime, err := time.Parse(time.RFC3339, tempOtp.CreatedAt)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to parse OTP creation time: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	expired := utils.IsExpiredOTP(createdAtTime, 3)
	if expired {
		return nil, &model.ApiError{
			Code:    401,
			Message: "OTP has expired",
			Type:    "unauthorized",
		}
	}

	_, err = ps.Appwrite.Users.DeleteTarget(userId, userId)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to delete target: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Update user's email in Appwrite Auth
	_, err = ps.Appwrite.Users.UpdateEmail(userId, tempOtp.Email)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to update user email in Appwrite Auth: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Update user's email verification status in Appwrite Auth

	_, err = ps.Appwrite.Users.UpdateEmailVerification(userId, true)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to update user email verification status: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Update user's email in the user collection document
	_, err = ps.Appwrite.Database.UpdateDocument(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.UsersCollectionID,
		userId,
		ps.Appwrite.Database.WithUpdateDocumentData(
			map[string]any{
				"email": tempOtp.Email,
			}),
	)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to update user email in database: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	// Delete the temporary OTP document
	_, err = ps.Appwrite.Database.DeleteDocument(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.TempOtpCollectionID,
		userId,
	)
	if err != nil {
		log.Printf("Failed to delete otp: %v", err.Error())
	}


	_,err = ps.Appwrite.Message.Delete(tempOtp.MessageId)
	if err != nil {
		log.Printf("Failed to delete message: %v", err.Error())
	}


	return &model.StatusOkay{Status: true, Message:tempOtp.Email}, nil
}


func (ps *GlobalService) SendOtp(ctx context.Context, payload *model.SendOtpPayload ,userId string) (*model.StatusOkay, *model.ApiError) {

	// Step1: Generate otp 
	messageId := id.Custom(uuid.NewString())
	subject := "Otp for " + payload.Subject + " verification"
	otp, err := utils.GenerateOTP()
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to generate OTP: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	content := "<p>Hello,<br>Please enter this code in the app to verify your identity. This code is valid for 3 minutes.Your One-Time Password (OTP) for verifying your identity is:<br><h1>" + otp + "</h1></p><p>Thank you,<br>ChatBasket</p>"

	emailTarget,err := ps.Appwrite.Users.ListTargets(userId)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to list targets: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	emailT := emailTarget.Targets[0].Id
	// if emailTarget.Total==1{
	// 	emailT=emailTarget.Targets[0].Id
	// }

	_, err = ps.Appwrite.Message.CreateEmail(
		messageId,
		subject,
		content,
		ps.Appwrite.Message.WithCreateEmailTargets([]string{emailT}),
	)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to send email: " + err.Error(),
			Type:    "internal_server_error",
		}
	}

	doc, err := ps.Appwrite.Database.ListDocuments(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.TempOtpCollectionID,
		ps.Appwrite.Database.WithListDocumentsQueries(
			[]string{
				query.Equal("userId", userId),
				query.Limit(1),
			},
		),
	)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to query otp data: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	if doc.Total == 1 {
		_, err = ps.Appwrite.Database.DeleteDocument(
			ps.Appwrite.DatabaseID,
			ps.Appwrite.TempOtpCollectionID,
			userId,
		)
		if err != nil {
			return nil, &model.ApiError{
				Code:    500,
				Message: "Failed to delete existing otp: " + err.Error(),
				Type:    "internal_server_error",
			}
		}
	}
	hashedOtp, err := utils.HashOTP(otp)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to hash OTP: " + err.Error(),
			Type:    "internal_server_error",
		}
	}
	tempOtpPayload := model.TempOtpPayload{
		Email:     payload.Subject,
		Otp:       hashedOtp,
		UserId:    userId,
		MessageId: messageId,
	}

	_, err = ps.Appwrite.Database.CreateDocument(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.TempOtpCollectionID,
		userId,
		tempOtpPayload,
	)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to save otp in database: " + err.Error(),
			Type:    "internal_server_error",		
		}
	}

	return &model.StatusOkay{Status: true, Message: "OTP sent to email"}, nil

}

func (ps *GlobalService) VerifyOtp(ctx context.Context, payload *model.OtpVerificationPayload, userId string) (*model.StatusOkay, *model.ApiError){
	// Step 1: Verify OTP
	
	// Retrieve the temporary OTP document from the database
	doc, err := ps.Appwrite.Database.GetDocument(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.TempOtpCollectionID,
		userId,
	)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to query otp data: "+err.Error(),
			Type:    "internal_server_error",
		}
	}
	var tempOtp model.TempOtp
	if err := doc.Decode(&tempOtp); err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to parse otp data: "+err.Error(),
			Type:    "internal_server_error",
		}
	}

	match, err := utils.VerifyOTP(payload.Secret, tempOtp.Otp)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to verify OTP: "+err.Error(),
			Type:    "internal_server_error",
		}
	}
	if !match {
		return nil, &model.ApiError{
			Code:    401,
			Message: "Invalid OTP",
			Type:    "unauthorized",
		}
	}

	// check if tempOtp has expired or not time limit is till 3 minutes after created at

	createdAtTime, err := time.Parse(time.RFC3339, tempOtp.CreatedAt)
	if err != nil {
		return nil, &model.ApiError{
			Code:    500,
			Message: "Failed to parse OTP creation time: "+err.Error(),
			Type:    "internal_server_error",
		}
	}

	expired := utils.IsExpiredOTP(createdAtTime, 3)
	if expired {
		return nil, &model.ApiError{
			Code:    401,
			Message: "OTP has expired",
			Type:    "unauthorized",
		}
	}

	// delete message but even it fails continue dont return nil
	_, err = ps.Appwrite.Message.Delete(tempOtp.MessageId)
	if err != nil {
		log.Printf("could not delete message: %v", err.Error())
	}

	_, err = ps.Appwrite.Database.DeleteDocument(
		ps.Appwrite.DatabaseID,
		ps.Appwrite.TempOtpCollectionID,
		userId,
	)
	if err != nil {
		log.Printf("Failed to delete otp: %v", err.Error())
	}
	return &model.StatusOkay{Status: true, Message: "OTP verified successfully"}, nil
}

