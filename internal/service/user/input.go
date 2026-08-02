package user

type CreateInput struct {
	Name     string
	Email    string
	Password string
}

type UpdateInput struct {
	Name  *string
	Email *string
}
