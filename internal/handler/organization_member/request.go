package organization_member

type UpdateOrganizationMemberRoleRequest struct {
	Role string `json:"role" validate:"required"`
}
