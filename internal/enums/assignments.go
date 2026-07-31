package enums

import "strings"

type assignment_mode string

const (
	inClinic assignment_mode = "IN_CLINIC"
	remote   assignment_mode = "REMOTE"
)

type AssignmentMode assignment_mode

type AssignmentModeDef struct {
	IN_CLINIC AssignmentMode
	REMOTE    AssignmentMode
}

var AssignmentModes = &AssignmentModeDef{
	IN_CLINIC: AssignmentMode(inClinic),
	REMOTE:    AssignmentMode(remote),
}

func (r AssignmentMode) String() string {
	return string(r)
}

func GetAssignmentModeFromString(assignmentMode string) AssignmentMode {
	switch strings.ToUpper(assignmentMode) {
	case "IN_CLINIC":
		return AssignmentModes.IN_CLINIC
	case "REMOTE":
		return AssignmentModes.REMOTE
	default:
		return AssignmentModes.IN_CLINIC
	}
}

func IsAssignmentModeValid(assignmentMode string) bool {
	switch strings.ToUpper(assignmentMode) {
	case "IN_CLINIC":
		return true
	case "REMOTE":
		return true
	default:
		return false
	}
}

type assignment_status string

const (
	assigned       assignment_status = "ASSIGNED"
	inProgress     assignment_status = "IN_PROGRESS"
	readyForReview assignment_status = "READY_FOR_REVIEW"
	completed      assignment_status = "COMPLETED"
)

type AssignmentStatus assignment_status

type AssignmentStatusDef struct {
	ASSIGNED         AssignmentStatus
	IN_PROGRESS      AssignmentStatus
	READY_FOR_REVIEW AssignmentStatus
	COMPLETED        AssignmentStatus
}

var AssignmentStatuses = &AssignmentStatusDef{
	ASSIGNED:         AssignmentStatus(assigned),
	IN_PROGRESS:      AssignmentStatus(inProgress),
	READY_FOR_REVIEW: AssignmentStatus(readyForReview),
	COMPLETED:        AssignmentStatus(completed),
}

func (r AssignmentStatus) String() string {
	return string(r)
}

func GetAssignmentStatusFromString(assignmentStatus string) AssignmentStatus {
	switch strings.ToUpper(assignmentStatus) {
	case string(assigned):
		return AssignmentStatuses.ASSIGNED
	case string(inProgress):
		return AssignmentStatuses.IN_PROGRESS
	case string(readyForReview):
		return AssignmentStatuses.READY_FOR_REVIEW
	case string(completed):
		return AssignmentStatuses.COMPLETED
	default:
		return AssignmentStatuses.ASSIGNED
	}
}

func IsAssignmentStatusValid(assignmentStatus string) bool {
	switch strings.ToUpper(assignmentStatus) {
	case string(assigned):
		return true
	case string(inProgress):
		return true
	case string(readyForReview):
		return true
	case string(completed):
		return true
	default:
		return false
	}
}
