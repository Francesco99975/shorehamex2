package models

import (
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"time"

	"github.com/Francesco99975/shorehamex2/internal/enums"
	"github.com/Francesco99975/shorehamex2/internal/repository"
)

type AssignPayload struct {
	PatientID   string   `form:"patient_id"`
	Instruments []string `form:"instruments"`
	Mode        string   `form:"mode"`
	DueDate     string   `form:"due_date"`
}

type AddPatientPayload struct {
	FullName    string         `form:"fullname"`
	Email       string         `form:"email"`
	Phone       string         `form:"phone"`
	DateOfBirth string         `form:"dob"`
	Sex         repository.Sex `form:"sex"`
}

func (p *AddPatientPayload) MakeDOBReadable() string {
	t, err := time.Parse("2006-01-02", p.DateOfBirth)
	if err != nil {
		return ""
	}
	return t.Format("January 2, 2006")
}

func (p *AddPatientPayload) DobToTime() time.Time {
	t, err := time.Parse("2006-01-02", p.DateOfBirth)
	if err != nil {
		return time.Time{}
	}
	return t
}

func (p *AddPatientPayload) NormalizePhone() (string, error) {
	// Keep only digits.
	digits := regexp.MustCompile(`\D`).ReplaceAllString(p.Phone, "")

	// Remove North American country code.
	if len(digits) == 11 && digits[0] == '1' {
		digits = digits[1:]
	}

	if len(digits) != 10 {
		return "", fmt.Errorf("invalid phone number: %q", p.Phone)
	}

	return fmt.Sprintf(
		"+1 (%s) %s-%s",
		digits[:3],
		digits[3:6],
		digits[6:],
	), nil
}

func (p *AddPatientPayload) Validate() error {
	if p.FullName == "" {
		return errors.New("fullname is required")
	}
	if p.Phone == "" {
		return errors.New("phone is required")
	}
	normalized, err := p.NormalizePhone()
	if err != nil {
		return err
	}
	p.Phone = normalized

	if p.Email == "" {
		return errors.New("email is required")
	}

	addr, err := mail.ParseAddress(p.Email)
	if err != nil {
		return errors.New("invalid email address")
	}
	if addr.Name != "" {
		return errors.New("invalid email address")
	}
	p.Email = addr.Address
	if p.DateOfBirth == "" {
		return errors.New("dob is required")
	}
	if p.MakeDOBReadable() == "" {
		return errors.New("invalid dob")
	}
	if p.Sex == "" {
		return errors.New("sex is required")
	}

	if !enums.IsSexValid(string(p.Sex)) {
		return errors.New("invalid sex")
	}
	return nil
}
