package jwks

import (
	"log/slog"
	"net/http"

	"github.com/disillusioned-labs/identity/internal/handler"
	jwkservice "github.com/disillusioned-labs/identity/internal/service/jwks"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var tracer = otel.Tracer("handler/jwks")

type JwksHandler struct {
	service jwkservice.JwksService
	log     *slog.Logger
}

func NewJwksHandler(service jwkservice.JwksService, log *slog.Logger) *JwksHandler {
	return &JwksHandler{service: service, log: log}
}

func (j *JwksHandler) jwks(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "JwksHandler.jwks")
	defer span.End()
	r = r.WithContext(ctx)

	output, err := j.service.Jwks(ctx)
	if err != nil {
		handler.WriteServiceError(w, r, j.log, err)
		return
	}
	span.SetAttributes(attribute.Int("len", len(output)))
	handler.WriteJSON(w, http.StatusOK, toJwksResponse(output))
}
