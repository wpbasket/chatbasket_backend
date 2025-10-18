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
// Pattern: 6 Uppercase letters(A-Z) + 4 digits(0-9) + 1 Uppercase letter(A-Z)
func GenerateRandomUsername() (string, error) {
 username := make([]byte, 11)

 // first 6 letters
 for i := 0; i < 6; i++ {
  idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
  if err != nil {
   return "", err
  }
  username[i] = letters[idx.Int64()]
 }

 // next 4 digits
 for i := 6; i < 10; i++ {
  idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
  if err != nil {
   return "", err
  }
  username[i] = digits[idx.Int64()]
 }

 // last letter
 idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
 if err != nil {
  return "", err
 }
 username[10] = letters[idx.Int64()]

 return string(username), nil
}

// ----------------------------
// Username Validation
// ----------------------------
// Checks if a username matches the pattern
// func ValidateUsername(username string) error {
//  pattern := `^[a-z]{6}[0-9]{4}[a-z]$`
//  match, err := regexp.MatchString(pattern, username)
//  if err != nil {
//   return err
//  }
//  if !match {
//   return errors.New("invalid username format: must be 6 letters + 4 digits + 1 letter")
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