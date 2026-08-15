package organization

type CreateRequest struct {
	Name string `json:"name" validate:"required,min=1,max=100"`
	Type string `json:"type" validate:"required"`
}

type UpdateRequest struct {
	Name string `json:"name" validate:"required,min=1,max=100"`
}
