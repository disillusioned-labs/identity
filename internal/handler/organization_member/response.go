package organization_member

import (
	"time"

	"github.com/google/uuid"

	organizationmemberservice "github.com/disillusioned-labs/identity/internal/service/organization_member"
)

type OrganizationMemberResponse struct {
	UserID   uuid.UUID `json:"user_id"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

func newOrganizationMemberResponse(
	member organizationmemberservice.OrganizationMemberOutput,
) OrganizationMemberResponse {
	return OrganizationMemberResponse{
		UserID:   member.UserID,
		Name:     member.Name,
		Email:    member.Email,
		Role:     member.Role,
		JoinedAt: member.JoinedAt,
	}
}

type ListOrganizationMembersResponse struct {
	Members []OrganizationMemberResponse `json:"members"`
}

func toListOrganizationMembersResponse(
	output organizationmemberservice.ListOrganizationMembersOutput,
) ListOrganizationMembersResponse {
	members := make([]OrganizationMemberResponse, 0, len(output.Members))

	for _, member := range output.Members {
		members = append(members, newOrganizationMemberResponse(member))
	}

	return ListOrganizationMembersResponse{
		Members: members,
	}
}

type UpdateOrganizationMemberRoleResponse struct {
	Member OrganizationMemberResponse `json:"member"`
}

func toUpdateOrganizationMemberRoleResponse(
	output organizationmemberservice.UpdateOrganizationMemberRoleOutput,
) UpdateOrganizationMemberRoleResponse {
	return UpdateOrganizationMemberRoleResponse{
		Member: newOrganizationMemberResponse(output.Member),
	}
}

type RemoveOrganizationMemberResponse struct {
	Removed bool `json:"removed"`
}

func toRemoveOrganizationMemberResponse(
	output organizationmemberservice.RemoveOrganizationMemberOutput,
) RemoveOrganizationMemberResponse {
	return RemoveOrganizationMemberResponse{
		Removed: true,
	}
}

type LeaveOrganizationResponse struct {
	OrganizationDeleted bool `json:"organization_deleted"`
}

func toLeaveOrganizationResponse(
	output organizationmemberservice.LeaveOrganizationOutput,
) LeaveOrganizationResponse {
	return LeaveOrganizationResponse{
		OrganizationDeleted: output.OrganizationDeleted,
	}
}
