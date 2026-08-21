package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/d31ma/sluice/internal/agent"
	"github.com/d31ma/sluice/internal/state/fylo"
)

type options struct {
	fyloBinary  string
	fyloRoot    string
	fyloVersion string
	apiToken    string
	sessionFile string
	listen      string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "Sluice agent could not start or shut down cleanly")
		os.Exit(1)
	}
}

func run(arguments []string) error {
	service, err := configuredService(arguments)
	if err != nil {
		return err
	}
	return runUntilSignal(service)
}

func configuredService(arguments []string) (*agent.Service, error) {
	config, err := parseOptions(arguments)
	if err != nil {
		return nil, err
	}
	token, err := readToken(config.apiToken)
	if err != nil {
		return nil, err
	}
	return newAgentService(config, token)
}

func newAgentService(config options, token string) (*agent.Service, error) {
	supervisor, err := fylo.NewSupervisor(fylo.Config{
		Binary:       config.fyloBinary,
		Root:         config.fyloRoot,
		Requirements: fylo.SluiceRequirements(config.fyloVersion),
	})
	if err != nil {
		return nil, fmt.Errorf("configure state runtime: %w", err)
	}
	return agent.NewService(agent.Config{
		State:       fyloOwner{supervisor: supervisor},
		Idempotency: agent.RejectingIdempotencyStore{},
		APIToken:    token,
		SessionFile: config.sessionFile,
		Listen:      config.listen,
	})
}

func runUntilSignal(service *agent.Service) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return service.Run(ctx)
}

func parseOptions(arguments []string) (options, error) {
	var config options
	flags := agentFlagSet(&config)
	if err := flags.Parse(arguments); err != nil {
		return options{}, fmt.Errorf("parse agent options")
	}
	return validateParsedOptions(flags, config)
}

func agentFlagSet(config *options) *flag.FlagSet {
	flags := flag.NewFlagSet("sluice-agent", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&config.fyloBinary, "fylo-binary", "", "absolute path to the pinned Fylo binary")
	flags.StringVar(&config.fyloRoot, "fylo-root", "", "absolute path to the private Fylo root")
	flags.StringVar(&config.fyloVersion, "fylo-version", "", "required Fylo runtime version")
	flags.StringVar(&config.apiToken, "api-token-file", "", "absolute path to the local API token")
	flags.StringVar(&config.sessionFile, "session-file", "", "absolute path to the private session descriptor")
	flags.StringVar(&config.listen, "listen", "127.0.0.1:0", "loopback agent IPC address")
	return flags
}

func validateParsedOptions(flags *flag.FlagSet, config options) (options, error) {
	if flags.NArg() != 0 {
		return options{}, fmt.Errorf("unexpected agent arguments")
	}
	for _, value := range requiredOptionValues(config) {
		if value == "" {
			return options{}, fmt.Errorf("required agent option is missing")
		}
	}
	return config, nil
}

func requiredOptionValues(config options) []string {
	return []string{
		config.fyloBinary,
		config.fyloRoot,
		config.fyloVersion,
		config.apiToken,
		config.sessionFile,
	}
}

func readToken(path string) (string, error) {
	clean, metadata, err := tokenFileInfo(path)
	if err != nil {
		return "", err
	}
	file, err := openVerifiedToken(clean, metadata)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	return readValidatedToken(file)
}

func tokenFileInfo(path string) (string, os.FileInfo, error) {
	clean, err := cleanTokenPath(path)
	if err != nil {
		return "", nil, err
	}
	metadata, err := lstatTokenFile(clean)
	if err != nil {
		return "", nil, err
	}
	return clean, metadata, validateTokenFileInfo(metadata)
}

func cleanTokenPath(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", fmt.Errorf("local API token file must be absolute")
	}
	return filepath.Clean(path), nil
}

func lstatTokenFile(path string) (os.FileInfo, error) {
	metadata, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("local API token file is unavailable")
	}
	return metadata, nil
}

func validateTokenFileInfo(metadata os.FileInfo) error {
	if !isRegularTokenFile(metadata) {
		return fmt.Errorf("local API token file is unavailable")
	}
	if !isPrivateTokenFile(metadata) {
		return fmt.Errorf("local API token file permissions are too broad")
	}
	return nil
}

func isRegularTokenFile(metadata os.FileInfo) bool {
	return metadata.Mode().IsRegular() && metadata.Mode()&os.ModeSymlink == 0
}

func isPrivateTokenFile(metadata os.FileInfo) bool {
	return runtime.GOOS == "windows" || metadata.Mode().Perm()&0o077 == 0
}

func openVerifiedToken(path string, pathInfo os.FileInfo) (*os.File, error) {
	// #nosec G304 -- the absolute, cleaned path was lstat'd as a private regular
	// file immediately above; it is an explicit administrator-supplied input.
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read local API token")
	}
	if err := verifyOpenedToken(file, pathInfo); err != nil {
		_ = file.Close()
		return nil, err
	}
	return file, nil
}

func verifyOpenedToken(file *os.File, pathInfo os.FileInfo) error {
	opened, err := file.Stat()
	if err != nil {
		return fmt.Errorf("local API token file changed while opening")
	}
	if !opened.Mode().IsRegular() || !os.SameFile(pathInfo, opened) {
		return fmt.Errorf("local API token file changed while opening")
	}
	return nil
}

func readValidatedToken(file *os.File) (string, error) {
	data, err := io.ReadAll(io.LimitReader(file, 4097))
	if err != nil || len(data) > 4096 {
		return "", fmt.Errorf("read local API token")
	}
	return validateToken(data)
}

func validateToken(data []byte) (string, error) {
	token := strings.TrimSpace(string(data))
	if len(token) < 32 {
		return "", errors.New("local API token is invalid")
	}
	return token, nil
}

type fyloOwner struct {
	supervisor *fylo.Supervisor
}

func (owner fyloOwner) Start(ctx context.Context) error {
	_, err := owner.supervisor.Start(ctx)
	return err
}

func (owner fyloOwner) Close(ctx context.Context) error {
	return owner.supervisor.Close(ctx)
}
