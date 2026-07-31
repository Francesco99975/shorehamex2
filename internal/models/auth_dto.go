package models

import (
	"errors"
	"net/mail"
	"strings"
	"unicode"

	"github.com/Francesco99975/shorehamex2/cmd/boot"
	"github.com/Francesco99975/shorehamex2/internal/enums"
)

type SignupRequest struct {
	Email    string `form:"email"`
	Username string `form:"username"`
	Password string `form:"password"`
	Confirm  string `form:"confirm"`
}

func (r *SignupRequest) ValidateAndNormalize(passwordSecurityLevel int) error {
	r.Email = strings.ToLower(strings.ReplaceAll(r.Email, " ", ""))
	r.Username = strings.ToLower(strings.ReplaceAll(r.Username, " ", ""))

	if r.Email == "" {
		return errors.New("email is required")
	}

	addr, err := mail.ParseAddress(r.Email)
	if err != nil {
		return errors.New("invalid email address")
	}
	if addr.Name != "" {
		return errors.New("invalid email address")
	}
	r.Email = addr.Address

	if r.Username == "" {
		return errors.New("username is required")
	}
	if r.Password == "" {
		return errors.New("password is required")
	}

	if strings.Contains(r.Password, " ") {
		return errors.New("password must not contain spaces")
	}

	if len(r.Password) > 128 {
		return errors.New("password must not exceed 128 characters")
	}

	for _, ch := range r.Password {
		// Reject null bytes and ASCII control characters
		if ch == 0x00 || (ch < 0x20 && ch != 0x09) || ch == 0x7F {
			return errors.New("password contains invalid characters")
		}
	}

	if len(r.Username) < 3 || len(r.Username) > 21 {
		return errors.New("username must be at least 3 characters and no more than 21 characters")
	}

	if boot.Environment.GoEnv == enums.Environments.PRODUCTION {

		switch passwordSecurityLevel {
		case 0:
			if len(r.Password) < 8 {
				return errors.New("password must be at least 8 characters")
			}
		case 1:
			if len(r.Password) < 8 {
				return errors.New("password must be at least 8 characters")
			}

			if !strings.ContainsAny(r.Password, "0123456789") {
				return errors.New("password must contain at least one number")
			}
		case 2:
			if len(r.Password) < 12 {
				return errors.New("password must be at least 12 characters")
			}

			if !strings.ContainsAny(r.Password, "0123456789") {
				return errors.New("password must contain at least one number")
			}

			if !containsSpecialChar(r.Password) {
				return errors.New("password must contain at least one special character: !@#$%")
			}
		}
	}

	if r.Password != r.Confirm {
		return errors.New("passwords do not match")
	}
	return nil

}

func containsSpecialChar(s string) bool {
	for _, ch := range s {
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) {
			return true
		}
	}
	return false
}

type LoginRequest struct {
	EmailOrUsername string `form:"eou"`
	Password        string `form:"password"`
	Remeber         string `form:"remember"`
}

func (r *LoginRequest) Validate() error {
	r.EmailOrUsername = strings.ToLower(strings.ReplaceAll(r.EmailOrUsername, " ", ""))
	r.Password = strings.ReplaceAll(r.Password, " ", "")

	if r.EmailOrUsername == "" {
		return errors.New("email or username is required")
	}
	if r.Password == "" {
		return errors.New("password is required")
	}
	return nil
}

type VerifyEmailRequest struct {
	Token string `form:"token"`
}

func (r *VerifyEmailRequest) Validate() error {
	if r.Token == "" {
		return errors.New("token is required")
	}
	return nil
}

type ResetPasswordRequest struct {
	Token    string `form:"token"`
	Password string `form:"new-password"`
	Confirm  string `form:"confirm-password"`
}

func (r *ResetPasswordRequest) Validate(passwordSecurityLevel int) error {
	if r.Token == "" {
		return errors.New("token is required")
	}
	if r.Password == "" {
		return errors.New("password is required")
	}
	if r.Password != r.Confirm {
		return errors.New("passwords do not match")
	}

	if boot.Environment.GoEnv == enums.Environments.PRODUCTION {

		switch passwordSecurityLevel {
		case 0:
			if len(r.Password) < 8 {
				return errors.New("password must be at least 8 characters")
			}
		case 1:
			if len(r.Password) < 8 {
				return errors.New("password must be at least 12 characters")
			}

			if !strings.ContainsAny(r.Password, "0123456789") {
				return errors.New("password must contain at least one number")
			}
		case 2:
			if len(r.Password) < 12 {
				return errors.New("password must be at least 16 characters")
			}

			if !strings.ContainsAny(r.Password, "0123456789") {
				return errors.New("password must contain at least one number")
			}

			if !strings.ContainsAny(r.Password, "!@#$%") {
				return errors.New("password must contain at least one special character: !@#$%")
			}
		}
	}
	return nil
}

type ChangeEmail struct {
	Username string `form:"username"`
	Email    string `form:"email"`
}

func (r *ChangeEmail) Validate() error {

	if r.Email == "" {
		return errors.New("email is required")
	}

	if boot.Environment.GoEnv == enums.Environments.PRODUCTION {
		if !strings.Contains(r.Email, "@") {
			return errors.New("invalid email")
		}
	}
	return nil
}

type ChangePasswordRequest struct {
	CurrentPassword string `form:"current_password"`
	NewPassword     string `form:"new_password"`
	Confirm         string `form:"confirm_password"`
}

func (r *ChangePasswordRequest) Validate(passwordSecurityLevel int) error {
	if r.CurrentPassword == "" {
		return errors.New("current password is required")
	}
	if r.NewPassword == "" {
		return errors.New("new password is required")
	}
	if r.NewPassword != r.Confirm {
		return errors.New("passwords do not match")
	}

	if boot.Environment.GoEnv == enums.Environments.PRODUCTION {
		switch passwordSecurityLevel {
		case 0:
			if len(r.NewPassword) < 8 {
				return errors.New("password must be at least 8 characters")
			}
		case 1:
			if len(r.NewPassword) < 8 {
				return errors.New("password must be at least 12 characters")
			}

			if !strings.ContainsAny(r.NewPassword, "0123456789") {
				return errors.New("password must contain at least one number")
			}
		case 2:
			if len(r.NewPassword) < 12 {
				return errors.New("password must be at least 16 characters")
			}

			if !strings.ContainsAny(r.NewPassword, "0123456789") {
				return errors.New("password must contain at least one number")
			}

			if !strings.ContainsAny(r.NewPassword, "!@#$%") {
				return errors.New("password must contain at least one special character: !@#$%")
			}
		}
	}

	return nil
}

type DisableTwoFARequest struct {
	Password string `form:"password"`
	Otp      string `form:"otp"`
}

func (r *DisableTwoFARequest) Validate() error {
	if r.Password == "" {
		return errors.New("password is required")
	}
	if r.Otp == "" {
		return errors.New("otp is required")
	}
	return nil
}

type CreateNewUser struct {
	Username string `form:"username"`
	Email    string `form:"email"`
	Password string `form:"password"`
	Role     string `form:"role"`
}

func (r *CreateNewUser) Validate(passwordSecurityLevel int) error {
	if r.Username == "" {
		return errors.New("username is required")
	}
	if r.Email == "" {
		return errors.New("email is required")
	}
	if r.Password == "" {
		return errors.New("password is required")
	}
	if r.Role == "" {
		return errors.New("role is required")
	}

	if !strings.Contains(r.Email, "@") {
		return errors.New("invalid email")
	}

	if boot.Environment.GoEnv == enums.Environments.PRODUCTION {
		switch passwordSecurityLevel {
		case 0:
			if len(r.Password) < 8 {
				return errors.New("password must be at least 8 characters")
			}
		case 1:
			if len(r.Password) < 8 {
				return errors.New("password must be at least 12 characters")
			}

			if !strings.ContainsAny(r.Password, "0123456789") {
				return errors.New("password must contain at least one number")
			}
		case 2:
			if len(r.Password) < 12 {
				return errors.New("password must be at least 16 characters")
			}

			if !strings.ContainsAny(r.Password, "0123456789") {
				return errors.New("password must contain at least one number")
			}

			if !strings.ContainsAny(r.Password, "!@#$%") {
				return errors.New("password must contain at least one special character: !@#$%")
			}
		}

		if !enums.IsRoleValid(r.Role) {
			return errors.New("invalid role")
		}
	}
	return nil
}

type UpdateUserRequest struct {
	Username string `form:"username"`
	Email    string `form:"email"`
	FullName string `form:"fullName"`
	Title    string `form:"title"`

	Password          string `form:"newPassword"`
	Role              string `form:"role"`
	Active            string `form:"active"`
	AdminAuthPassword string `form:"adminPassword"`
}

func (r *UpdateUserRequest) Validate(passwordSecurityLevel int) error {
	if r.Username == "" {
		return errors.New("username is required")
	}
	if r.Email == "" {
		return errors.New("email is required")
	}
	if r.Role == "" {
		return errors.New("role is required")
	}

	if !strings.Contains(r.Email, "@") {
		return errors.New("invalid email")
	}

	if r.Password != "" {
		if strings.Contains(r.Password, " ") {
			return errors.New("password must not contain spaces")
		}

		if boot.Environment.GoEnv == enums.Environments.PRODUCTION {
			switch passwordSecurityLevel {
			case 0:
				if len(r.Password) < 8 {
					return errors.New("password must be at least 8 characters")
				}
			case 1:
				if len(r.Password) < 8 {
					return errors.New("password must be at least 12 characters")
				}

				if !strings.ContainsAny(r.Password, "0123456789") {
					return errors.New("password must contain at least one number")
				}
			case 2:
				if len(r.Password) < 12 {
					return errors.New("password must be at least 16 characters")
				}

				if !strings.ContainsAny(r.Password, "0123456789") {
					return errors.New("password must contain at least one number")
				}

				if !strings.ContainsAny(r.Password, "!@#$%") {
					return errors.New("password must contain at least one special character: !@#$%")
				}
			}

			if !enums.IsRoleValid(r.Role) {
				return errors.New("invalid role")
			}
		}
	}
	return nil
}
