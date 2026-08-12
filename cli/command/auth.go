package command

import (
	"bufio"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/bsenel/karakuri/cli/client"
	"github.com/spf13/cobra"
)

func authCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authenticate and manage principals, roles and bindings",
		Long: `Karakuri issues short-lived access tokens backed by a rotating refresh
token. ` + "`krk auth login`" + ` caches both under ~/.config/karakuri/credentials.json
(override with KARAKURI_CREDENTIALS); every other command refreshes them
automatically, so you log in once rather than per command.`,
	}
	cmd.AddCommand(
		authLoginCmd(),
		authLogoutCmd(),
		authWhoamiCmd(),
		authUsersCmd(),
		authRolesCmd(),
		authPoliciesCmd(),
		authBindingsCmd(),
		authCheckCmd(),
		authCatalogCmd(),
	)
	return cmd
}

func authLoginCmd() *cobra.Command {
	var (
		id            string
		passwordStdin bool
		refreshToken  string
		sso           bool
		noBrowser     bool
	)
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in and cache credentials for this API server",
		Long: `Reads the password from stdin with --password-stdin, which keeps it out of
your shell history and out of the process list. Service accounts have no
password; pass the refresh token they were issued with --refresh-token.

With --sso, logging in goes through the identity provider this server is
configured for: a browser opens, and the credential comes back to a listener on
localhost. Nothing usable travels through the browser — the code it carries is
bound to a secret that never leaves this process. Use --no-browser on a machine
with no browser to open and paste the URL somewhere else.

Password login keeps working when a provider is configured. That is deliberate:
it is how an administrator gets in when the identity provider is down.`,
		Example: `  krk auth login --id admin --password-stdin < password.txt
  echo "$PASSWORD" | krk auth login --id alice --password-stdin
  krk auth login --refresh-token "$CI_TOKEN"
  krk auth login --sso
  krk auth login --sso --no-browser`,
		RunE: func(c *cobra.Command, _ []string) error {
			if sso {
				session, err := api.SSOLogin(c.Context(), func(target string) {
					fmt.Fprintf(c.OutOrStdout(), "Opening %s\n", target)
				}, !noBrowser)
				if err != nil {
					return err
				}
				fmt.Fprintf(c.OutOrStdout(), "Logged in to %s as %s\n",
					api.BaseURL, orDefault(session.PrincipalID, "unknown principal"))
				return nil
			}
			if refreshToken != "" {
				session, err := api.LoginWithRefreshToken(refreshToken)
				if err != nil {
					return err
				}
				fmt.Fprintf(c.OutOrStdout(), "Logged in to %s as %s\n", api.BaseURL, orDefault(session.PrincipalID, "service account"))
				return nil
			}
			if id == "" {
				return fmt.Errorf("--id is required (or use --refresh-token)")
			}
			if !passwordStdin {
				return fmt.Errorf("--password-stdin is required: passing a password as a flag would put it in your shell history")
			}
			password, err := readSecret(c.InOrStdin())
			if err != nil {
				return err
			}
			if _, err := api.Login(id, password); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "Logged in to %s as %s\n", api.BaseURL, id)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "Principal ID to log in as")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "Read the password from stdin")
	cmd.Flags().StringVar(&refreshToken, "refresh-token", "", "Adopt a refresh token issued for a service account")
	cmd.Flags().BoolVar(&sso, "sso", false, "Log in through the server's identity provider in a browser")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "With --sso, print the URL instead of opening a browser")
	return cmd
}

func authLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Revoke this session and forget the cached credentials",
		RunE: func(c *cobra.Command, _ []string) error {
			if err := api.Logout(); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "Logged out of %s\n", api.BaseURL)
			return nil
		},
	}
}

func authWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the current principal, its roles and effective permissions",
		RunE: func(_ *cobra.Command, _ []string) error {
			data, _, err := api.Get("/auth/me")
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
}

func authUsersCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "users", Short: "Manage principals"}

	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List principals",
		RunE: func(_ *cobra.Command, _ []string) error {
			data, _, err := api.Get("/auth/users")
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	})

	var (
		id             string
		name           string
		roles          []string
		scope          string
		serviceAccount bool
		passwordStdin  bool
	)
	add := &cobra.Command{
		Use:   "add",
		Short: "Create a principal and bind it to roles",
		Long: `Users authenticate with a password; service accounts (--service-account) get
a refresh token printed once and never again, since only its hash is stored.

--scope limits the role binding to one resource, so "operator on twin:abc" is
expressible without granting operator everywhere.`,
		Example: `  echo "$PW" | krk auth users add --id alice --roles operator --password-stdin
  krk auth users add --id ci --roles operator --service-account
  echo "$PW" | krk auth users add --id bob --roles viewer --scope twin:abc --password-stdin`,
		RunE: func(c *cobra.Command, _ []string) error {
			if id == "" {
				return fmt.Errorf("--id is required")
			}
			body := map[string]any{"id": id, "name": name, "roles": roles}
			if scope != "" {
				body["scope"] = scope
			}
			if serviceAccount {
				body["service_account"] = true
			} else {
				if !passwordStdin {
					return fmt.Errorf("--password-stdin is required for a user (or use --service-account)")
				}
				password, err := readSecret(c.InOrStdin())
				if err != nil {
					return err
				}
				body["password"] = password
			}
			data, _, err := api.Post("/auth/users", body)
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			if serviceAccount && output != "json" {
				fmt.Fprintln(c.ErrOrStderr(),
					"\nThe refresh token above is shown once — only its hash is stored. Save it now.")
			}
			return nil
		},
	}
	add.Flags().StringVar(&id, "id", "", "Principal ID")
	add.Flags().StringVar(&name, "name", "", "Display name")
	add.Flags().StringSliceVar(&roles, "roles", nil, "Roles to bind (repeatable or comma-separated)")
	add.Flags().StringVar(&scope, "scope", "", "Limit the binding to a resource, e.g. twin:abc (default: everything)")
	add.Flags().BoolVar(&serviceAccount, "service-account", false, "Create a machine identity with a refresh token instead of a password")
	add.Flags().BoolVar(&passwordStdin, "password-stdin", false, "Read the password from stdin")
	cmd.AddCommand(add)
	return cmd
}

func authRolesCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "roles", Short: "Inspect roles"}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List roles with their policies",
		RunE: func(_ *cobra.Command, _ []string) error {
			data, _, err := api.Get("/auth/roles")
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	})
	return cmd
}

func authPoliciesCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "policies", Short: "Inspect policies"}
	var principal string
	list := &cobra.Command{
		Use:   "list",
		Short: "List every policy, or the effective grants reaching one principal",
		RunE: func(_ *cobra.Command, _ []string) error {
			path := "/auth/policies"
			if principal != "" {
				path += "?" + url.Values{"principal": {principal}}.Encode()
			}
			data, _, err := api.Get(path)
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	list.Flags().StringVar(&principal, "principal", "", "Show the grants that reach this principal, flattened across role inheritance")
	cmd.AddCommand(list)
	return cmd
}

func authBindingsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "bindings", Short: "Manage role bindings"}
	var (
		principal string
		role      string
		scope     string
		org       string
		team      string
		project   string
	)
	add := &cobra.Command{
		Use:   "add",
		Short: "Grant a principal a role, optionally over one resource or container",
		Example: `  krk auth bindings add --principal alice --role operator --scope twin:abc
  krk auth bindings add --principal alice --role operator --org acme --team eng
  krk auth bindings add --principal oidc:bob --role viewer --project delta`,
		RunE: func(_ *cobra.Command, _ []string) error {
			if principal == "" || role == "" {
				return fmt.Errorf("--principal and --role are required")
			}
			// A container named on the command line becomes the scope. It is
			// resolved to an ID here, so what reaches the binding is
			// "team:t_7f2a" and never the word "eng" — two organisations may
			// each have a team by that name, and a grant matching on the word
			// would cover both.
			resolved, err := containerScope(org, team, project)
			if err != nil {
				return err
			}
			if resolved != "" {
				if scope != "" {
					return fmt.Errorf("--scope and --org/--team/--project name the same thing; use one")
				}
				scope = resolved
			}
			data, _, err := api.Post("/auth/bindings", map[string]any{
				"principal_id": principal, "role": role, "scope": scope,
			})
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	add.Flags().StringVar(&principal, "principal", "", "Principal ID")
	add.Flags().StringVar(&role, "role", "", "Role name")
	add.Flags().StringVar(&scope, "scope", "", "Resource scope, e.g. twin:abc (default: everything)")
	add.Flags().StringVar(&org, "org", "", "Grant over an organisation and everything inside it")
	add.Flags().StringVar(&team, "team", "", "Grant over a team; needs --org")
	add.Flags().StringVar(&project, "project", "", "Grant over a project")
	cmd.AddCommand(add)
	return cmd
}

func authCheckCmd() *cobra.Command {
	var owner string
	cmd := &cobra.Command{
		Use:   "check <principal> <action> <resource>",
		Short: "Ask whether a principal may do something, and why",
		Long: `Returns the full decision trace: the matched policy, the role it came
through, the binding scope in play, and how each condition evaluated. Use it to
explain a 403 without reading the policy table by hand.`,
		Example: `  krk auth check vera loop:start 'loop:*'
  krk auth check alice twin:update twin:abc --owner alice`,
		Args: cobra.ExactArgs(3),
		RunE: func(_ *cobra.Command, args []string) error {
			body := map[string]any{"principal": args[0], "action": args[1], "resource": args[2]}
			if owner != "" {
				body["owner"] = owner
			}
			data, _, err := api.Post("/auth/check", body)
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
	cmd.Flags().StringVar(&owner, "owner", "", "Treat the resource as owned by this principal (exercises owner_equals conditions)")
	return cmd
}

func authCatalogCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "catalog",
		Short: "List every action the server recognises",
		RunE: func(_ *cobra.Command, _ []string) error {
			data, _, err := api.Get("/auth/catalog")
			if err != nil {
				return err
			}
			client.PrintOutput(data, output)
			return nil
		},
	}
}

func orDefault(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// readSecret reads a single line from stdin, trimming the trailing newline a
// pipe or heredoc adds. Passwords never come from flags: those land in shell
// history and in the process list.
func readSecret(r io.Reader) (string, error) {
	reader := bufio.NewReader(r)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read from stdin: %w", err)
	}
	secret := strings.TrimRight(line, "\r\n")
	if secret == "" {
		return "", fmt.Errorf("no password on stdin")
	}
	return secret, nil
}
