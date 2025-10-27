package personalutils

import (
 "crypto/rand"
 "math/big"
)

// ----------------------------
// Character sets
// ----------------------------
const (
 letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
 digits  = "0123456789"
)

// ----------------------------
// GenerateRandomUsername
// ----------------------------
// Pattern: 4 Uppercase letters(A-Z) + 6 digits(0-9)
func GenerateRandomUsername() (string, error) {
 username := make([]byte, 10)

 // first 4 letters
 for i := 0; i < 4; i++ {
  idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
  if err != nil {
   return "", err
  }
  username[i] = letters[idx.Int64()]
 }

 // next 6 digits
 for i := 4; i < 10; i++ {
  idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
  if err != nil {
   return "", err
  }
  username[i] = digits[idx.Int64()]
 }

 return string(username), nil
}

// ----------------------------
// Username Validation
// ----------------------------
// Checks if a username matches the pattern
// func ValidateUsername(username string) error {
//  pattern := `^[A-Z]{4}[0-9]{6}$`
//  match, err := regexp.MatchString(pattern, username)
//  if err != nil {
//   return err
//  }
//  if !match {
//   return errors.New("invalid username format: must be 4 uppercase letters + 6 digits")
//  }
//  return nil
// }

// ----------------------------
// Custom validator registration
// ----------------------------
// func RegisterCreateUserProfileValidators(v *validator.Validate) error {
//  return v.RegisterValidation("username", func(fl validator.FieldLevel) bool {
//   username := fl.Field().String()
//   err := ValidateUsername(username)
//   return err == nil
//  })
// }