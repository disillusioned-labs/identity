package organization

import (
	organizationservice "github.com/disillusioned-labs/identity/internal/service/organization"
)

type OrganizationResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Role string `json:"role"`
}

func newOrganizationResponse(organization organizationservice.OrganizationOutput) OrganizationResponse {
	return OrganizationResponse{
		ID:   organization.ID.String(),
		Name: organization.Name,
		Type: organization.Type,
		Role: organization.Role,
	}
}

type ListResponse struct {
	Organizations []OrganizationResponse `json:"organizations"`
}

func toListResponse(output organizationservice.ListOutput) ListResponse {
	organizations := make([]OrganizationResponse, 0, len(output.Organizations))

	for _, organization := range output.Organizations {
		organizations = append(organizations, newOrganizationResponse(organization))
	}

	return ListResponse{
		Organizations: organizations,
	}
}

type CreateResponse struct {
	Organization OrganizationResponse `json:"organization"`
}

func toCreateResponse(output organizationservice.CreateOutput) CreateResponse {
	return CreateResponse{
		Organization: newOrganizationResponse(output.Organization),
	}
}

type GetResponse struct {
	Organization OrganizationResponse `json:"organization"`
}

func toGetResponse(output organizationservice.GetOutput) GetResponse {
	return GetResponse{
		Organization: newOrganizationResponse(output.Organization),
	}
}

type UpdateResponse struct {
	Organization OrganizationResponse `json:"organization"`
}

func toUpdateResponse(output organizationservice.UpdateOutput) UpdateResponse {
	return UpdateResponse{
		Organization: newOrganizationResponse(output.Organization),
	}
}

func toDeleteResponse(output organizationservice.DeleteOutput) struct{} {
	return struct{}{}
}

type TransferUserResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

type TransferResponse struct {
	Organization OrganizationResponse `json:"organization"`
	From         TransferUserResponse `json:"from"`
	To           TransferUserResponse `json:"to"`
	Notice       string               `json:"notice"`
}

func toTransferResponse(output organizationservice.TransferOutput) TransferResponse {
	return TransferResponse{
		Organization: newOrganizationResponse(output.Organization),
		From: TransferUserResponse{
			ID:   output.From.ID.String(),
			Name: output.From.Name,
			Role: output.From.Role,
		},
		To: TransferUserResponse{
			ID:   output.To.ID.String(),
			Name: output.To.Name,
			Role: output.To.Role,
		},
		Notice: "approval rules are not transferred; they belong to the expense service",
	}
}
