package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ysfl/baize-mcp/internal/baize"
	"github.com/ysfl/baize-mcp/internal/buildinfo"
	"github.com/ysfl/baize-mcp/internal/credential"
	"github.com/ysfl/baize-mcp/internal/mcpserver"
	"github.com/ysfl/baize-mcp/internal/profile"
)

const defaultProfile = "default"

func Run(ctx context.Context, args []string, stdin *os.File, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}
	store, err := profile.NewDefaultStore()
	if err != nil {
		return fail(stderr, err)
	}
	credentials := credential.NewKeyringStore()

	switch args[0] {
	case "login":
		err = runLogin(ctx, args[1:], stdin, stdout, stderr, store, credentials)
	case "status":
		err = runStatus(ctx, args[1:], stdout, stderr, store, credentials)
	case "logout":
		err = runLogout(ctx, args[1:], stdout, stderr, store, credentials)
	case "serve":
		err = runServe(ctx, args[1:], stdin, stdout, stderr, store, credentials)
	case "config-path":
		if len(args) != 1 {
			err = errors.New("config-path does not accept arguments")
		} else {
			_, err = fmt.Fprintln(stdout, store.Path())
		}
	case "version", "--version", "-version":
		_, err = fmt.Fprintln(stdout, buildinfo.Version)
	case "help", "--help", "-h":
		printUsage(stdout)
	default:
		err = fmt.Errorf("unknown command %q", args[0])
	}
	if err != nil {
		return fail(stderr, err)
	}
	return 0
}

func runServe(ctx context.Context, args []string, stdin io.ReadCloser, stdout io.Writer, stderr io.Writer, profiles *profile.Store, credentials credential.Store) error {
	flags := flag.NewFlagSet("serve", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profileName := flags.String("profile", defaultProfile, "local profile name")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("serve received unexpected positional arguments")
	}
	client, _, err := authenticatedClient(*profileName, profiles, credentials)
	if err != nil {
		return err
	}
	server := mcpserver.New(client)
	transport := &mcp.IOTransport{Reader: stdin, Writer: noCloseWriter{Writer: stdout}}
	if err := server.Run(ctx, transport); err != nil && !errors.Is(err, context.Canceled) {
		return fmt.Errorf("run MCP server: %w", err)
	}
	return nil
}

func runLogin(ctx context.Context, args []string, stdin *os.File, stdout, stderr io.Writer, profiles *profile.Store, credentials credential.Store) error {
	flags := flag.NewFlagSet("login", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profileName := flags.String("profile", defaultProfile, "local profile name")
	apiURL := flags.String("api-url", "", "Baize API URL ending in /api/v1")
	username := flags.String("username", "", "Baize username")
	allowHTTP := flags.Bool("allow-http", false, "allow non-loopback HTTP for this profile")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("login received unexpected positional arguments")
	}
	if err := profile.ValidateName(*profileName); err != nil {
		return err
	}
	if strings.TrimSpace(*apiURL) == "" || strings.TrimSpace(*username) == "" {
		return errors.New("login requires --api-url and --username")
	}
	if !term.IsTerminal(int(stdin.Fd())) {
		return errors.New("login requires an interactive terminal so the password is not passed as an argument")
	}
	normalizedURL, err := baize.ValidateAPIURL(*apiURL, *allowHTTP)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprint(stderr, "Password: "); err != nil {
		return err
	}
	password, err := term.ReadPassword(int(stdin.Fd()))
	_, _ = fmt.Fprintln(stderr)
	if err != nil {
		return errors.New("read password from terminal")
	}
	defer clearBytes(password)

	client, err := baize.NewClient(normalizedURL, "", *allowHTTP, userAgent())
	if err != nil {
		return err
	}
	requestCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	token, err := client.Login(requestCtx, strings.TrimSpace(*username), string(password))
	if err != nil {
		return err
	}

	oldToken, oldTokenErr := credentials.Get(*profileName)
	if oldTokenErr != nil && !errors.Is(oldTokenErr, credential.ErrNotFound) {
		return oldTokenErr
	}
	if err := credentials.Set(*profileName, token); err != nil {
		return err
	}
	item := profile.Profile{APIURL: normalizedURL, AllowHTTP: *allowHTTP}
	if err := profiles.Put(*profileName, item); err != nil {
		if oldTokenErr == nil {
			_ = credentials.Set(*profileName, oldToken)
		} else {
			_ = credentials.Delete(*profileName)
		}
		return err
	}
	return writeJSON(stdout, map[string]any{"authenticated": true})
}

func runStatus(ctx context.Context, args []string, stdout, stderr io.Writer, profiles *profile.Store, credentials credential.Store) error {
	flags := flag.NewFlagSet("status", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profileName := flags.String("profile", defaultProfile, "local profile name")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("status received unexpected positional arguments")
	}
	client, _, err := authenticatedClient(*profileName, profiles, credentials)
	if err != nil {
		return err
	}
	requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := client.CheckSession(requestCtx); err != nil {
		return err
	}
	return writeJSON(stdout, map[string]any{"authenticated": true})
}

func runLogout(ctx context.Context, args []string, stdout, stderr io.Writer, profiles *profile.Store, credentials credential.Store) error {
	flags := flag.NewFlagSet("logout", flag.ContinueOnError)
	flags.SetOutput(stderr)
	profileName := flags.String("profile", defaultProfile, "local profile name")
	localOnly := flags.Bool("local-only", false, "remove only the local session credential")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("logout received unexpected positional arguments")
	}
	if !*localOnly {
		client, _, err := authenticatedClient(*profileName, profiles, credentials)
		if err != nil {
			return err
		}
		requestCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		err = client.Logout(requestCtx)
		cancel()
		var apiErr *baize.APIError
		if err != nil && (!errors.As(err, &apiErr) || apiErr.StatusCode != 401) {
			return err
		}
	}
	if err := credentials.Delete(*profileName); err != nil && !errors.Is(err, credential.ErrNotFound) {
		return err
	}
	return writeJSON(stdout, map[string]any{"authenticated": false})
}

func authenticatedClient(profileName string, profiles *profile.Store, credentials credential.Store) (*baize.Client, profile.Profile, error) {
	if err := profile.ValidateName(profileName); err != nil {
		return nil, profile.Profile{}, err
	}
	item, err := profiles.Get(profileName)
	if err != nil {
		return nil, profile.Profile{}, err
	}
	token, err := credentials.Get(profileName)
	if err != nil {
		return nil, profile.Profile{}, err
	}
	client, err := baize.NewClient(item.APIURL, token, item.AllowHTTP, userAgent())
	if err != nil {
		return nil, profile.Profile{}, err
	}
	return client, item, nil
}

func userAgent() string {
	return "baize-mcp/" + buildinfo.Version
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(true)
	return encoder.Encode(value)
}

func printUsage(w io.Writer) {
	_, _ = fmt.Fprintln(w, "Usage: baize-mcp <login|status|logout|serve|config-path|version>")
}

func fail(w io.Writer, err error) int {
	_, _ = fmt.Fprintf(w, "error: %s\n", err)
	return 1
}

func clearBytes(value []byte) {
	for i := range value {
		value[i] = 0
	}
}

type noCloseWriter struct {
	io.Writer
}

func (noCloseWriter) Close() error {
	return nil
}
