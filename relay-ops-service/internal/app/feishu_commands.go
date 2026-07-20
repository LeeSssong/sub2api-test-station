package app

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"example.invalid/relay-ops-service/internal/commands"
	"example.invalid/relay-ops-service/internal/config"
	"example.invalid/relay-ops-service/internal/feishuapi"
	"example.invalid/relay-ops-service/internal/feishuevents"
	"example.invalid/relay-ops-service/internal/routingcontrol"
	"example.invalid/relay-ops-service/internal/sub2api"
)

const feishuOpenAPIBaseURL = "https://open.feishu.cn"

type FeishuCommandRepository interface {
	InsertFeishuEvent(context.Context, commands.Record) (bool, error)
	ClaimFeishuCommand(context.Context, time.Time, time.Duration) (*commands.Record, error)
	CompleteFeishuCommand(context.Context, commands.Completion) error
	RecordFeishuReply(context.Context, string, string, bool, string) error
	WithFeishuGroupLock(context.Context, int64, func(context.Context) commands.Completion) (commands.Completion, error)
}

type FeishuCommandDependencies struct {
	Repository FeishuCommandRepository
	Sub2API    sub2api.Controller
}

type FeishuCommandRuntime struct {
	Handler http.Handler
	Worker  *commands.Worker
}

func AttachFeishuCommandHandler(fallback, callback http.Handler) http.Handler {
	if callback == nil {
		return fallback
	}
	mux := http.NewServeMux()
	mux.Handle("POST /relay-ops/api/feishu/events", callback)
	mux.Handle("/", fallback)
	return mux
}

func mountHandlers(fallback, callback http.Handler) http.Handler {
	return AttachFeishuCommandHandler(fallback, callback)
}

func ConfigureFeishuCommands(cfg config.Config, dependencies *FeishuCommandDependencies, fallback http.Handler) (FeishuCommandRuntime, error) {
	if fallback == nil {
		return FeishuCommandRuntime{}, errors.New("Feishu command fallback handler is unavailable")
	}
	if cfg.FeishuAppIDFile == "" && cfg.FeishuAppSecretFile == "" && cfg.FeishuVerificationFile == "" && cfg.FeishuEncryptKeyFile == "" && cfg.FeishuRoutingFile == "" {
		if cfg.FeishuCommandMode != "" && cfg.FeishuCommandMode != config.FeishuCommandDisabled {
			return FeishuCommandRuntime{}, errors.New("Feishu command configuration is incomplete")
		}
		return FeishuCommandRuntime{Handler: fallback}, nil
	}
	if dependencies == nil || dependencies.Repository == nil || (cfg.FeishuCommandMode != config.FeishuCommandDisabled && dependencies.Sub2API == nil) {
		return FeishuCommandRuntime{}, errors.New("Feishu command dependencies are unavailable")
	}
	verificationToken, err := readFeishuCommandSecret(cfg.FeishuVerificationFile)
	if err != nil {
		return FeishuCommandRuntime{}, errors.New("Feishu verification token is unavailable")
	}
	encryptKey, err := readFeishuCommandSecret(cfg.FeishuEncryptKeyFile)
	if err != nil {
		return FeishuCommandRuntime{}, errors.New("Feishu Encrypt Key is unavailable")
	}
	verifier, err := feishuevents.NewVerifier(verificationToken, encryptKey, time.Now)
	if err != nil {
		return FeishuCommandRuntime{}, errors.New("Feishu callback verifier is unavailable")
	}
	sender, err := feishuapi.NewClient(feishuOpenAPIBaseURL, cfg.FeishuAppIDFile, cfg.FeishuAppSecretFile)
	if err != nil {
		return FeishuCommandRuntime{}, errors.New("Feishu reply client is unavailable")
	}
	var router commands.Router
	var groupIDs map[string]int64
	if cfg.FeishuCommandMode != config.FeishuCommandDisabled {
		routingConfig, err := routingcontrol.LoadConfig(cfg.FeishuRoutingFile)
		if err != nil {
			return FeishuCommandRuntime{}, errors.New("Feishu routing configuration is invalid")
		}
		router = &routingcontrol.Controller{Client: dependencies.Sub2API, Config: routingConfig}
		groupIDs = make(map[string]int64, len(routingConfig.Groups))
		for _, group := range routingConfig.Groups {
			groupIDs[group.Name] = group.PublicGroupID
		}
	}
	callback := commands.NewHTTPHandler(verifier, dependencies.Repository, time.Now)
	worker := &commands.Worker{
		Mode: cfg.FeishuCommandMode, Repository: dependencies.Repository, Router: router,
		Sender: sender, GroupIDs: groupIDs,
	}
	return FeishuCommandRuntime{Handler: AttachFeishuCommandHandler(fallback, callback), Worker: worker}, nil
}

func ConfigureFeishuCommandsForStore(cfg config.Config, repository FeishuCommandRepository, fallback http.Handler) (FeishuCommandRuntime, error) {
	if cfg.FeishuAppIDFile == "" && cfg.FeishuAppSecretFile == "" && cfg.FeishuVerificationFile == "" && cfg.FeishuEncryptKeyFile == "" && cfg.FeishuRoutingFile == "" {
		return ConfigureFeishuCommands(cfg, nil, fallback)
	}
	if cfg.FeishuCommandMode == config.FeishuCommandDisabled {
		return ConfigureFeishuCommands(cfg, &FeishuCommandDependencies{Repository: repository}, fallback)
	}
	reader, err := sub2api.NewHTTPReader(cfg.Sub2APIBaseURL, cfg.Sub2APIAdminKeyFile)
	if err != nil {
		return FeishuCommandRuntime{}, errors.New("Feishu Sub2API controller is unavailable")
	}
	return ConfigureFeishuCommands(cfg, &FeishuCommandDependencies{Repository: repository, Sub2API: reader}, fallback)
}

func readFeishuCommandSecret(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 64<<10 {
		return "", errors.New("secret is unavailable")
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return "", errors.New("secret is empty")
	}
	return string(data), nil
}
