// Command accred is a chat terminal for Accred's tokenized LLM credits.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/useAccred/accred-cli/internal/api"
	"github.com/useAccred/accred-cli/internal/config"
	"github.com/useAccred/accred-cli/internal/tui"
	"golang.org/x/term"
)

// version is set by scripts/build-release.sh.
var version = "0.1.0"

const usage = `accred — tokenized LLM credits in your terminal

Usage:
  accred                     open the chat terminal
  accred -p "prompt"         print one reply and exit (also reads the prompt from a pipe)
  accred login               save your API key in the system keychain (or use /login inside)
  accred logout              remove the saved API key
  accred models [filter]     list available models and prices

Flags:
  -p string          prompt for a single reply
  --model string     model id to use (default: the last one you picked)
  --max-tokens int   most tokens a reply may use, 1 to 8192
  --no-mascot        hide the mascot
  --version          print the version

The API key is read from ACCRED_API_KEY, then from the system keychain.
Create one on the API page at https://accred.sh
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "accred:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) > 0 {
		switch args[0] {
		case "login":
			return login()
		case "logout":
			if err := config.DeleteAPIKey(); err != nil {
				return err
			}
			fmt.Println("Removed the saved API key.")
			return nil
		case "models":
			return listModels(strings.Join(args[1:], " "))
		case "help":
			fmt.Print(usage)
			return nil
		case "version":
			fmt.Println(version)
			return nil
		}
	}

	flags := flag.NewFlagSet("accred", flag.ContinueOnError)
	flags.Usage = func() { fmt.Fprint(os.Stderr, usage) }
	prompt := flags.String("p", "", "")
	model := flags.String("model", "", "")
	maxTokens := flags.Int("max-tokens", 0, "")
	noMascot := flags.Bool("no-mascot", false, "")
	showVersion := flags.Bool("version", false, "")
	if err := flags.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return nil
		}
		return err
	}
	if *showVersion {
		fmt.Println(version)
		return nil
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unknown command %q. Run `accred help` for usage", flags.Arg(0))
	}
	if *maxTokens < 0 || *maxTokens > 8192 {
		return fmt.Errorf("--max-tokens must be from 1 to 8192")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("could not read preferences: %w", err)
	}
	if *model != "" {
		cfg.Model = *model
	}
	if *maxTokens > 0 {
		cfg.MaxTokens = *maxTokens
	}
	if *noMascot {
		cfg.NoMascot = true
	}

	key, source := config.APIKey()
	client := api.New(key)

	piped := !term.IsTerminal(int(os.Stdin.Fd()))
	if *prompt != "" || piped {
		if key == "" {
			return fmt.Errorf("no API key found. Run `accred login`, or set %s", config.EnvAPIKey)
		}
		text := *prompt
		if piped {
			raw, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			text = strings.TrimSpace(text + "\n" + string(raw))
		}
		return printReply(client, cfg, text)
	}

	app := tui.New(tui.Options{
		Backend:   client,
		Config:    cfg,
		KeySource: source,
		Version:   version,
		Dark:      lipgloss.HasDarkBackground(),
		Save:      config.Save,
		HasKey:    key != "",
		Login: func(ctx context.Context, key string) error {
			if err := verifyAndSaveKey(ctx, key); err != nil {
				return err
			}
			client.APIKey = key
			return nil
		},
		Logout: func() error {
			client.APIKey = ""
			return config.DeleteAPIKey()
		},
	})
	_, err = tea.NewProgram(app).Run()
	return err
}

func printReply(client *api.Client, cfg *config.Config, prompt string) error {
	if prompt == "" {
		return fmt.Errorf("the prompt is empty")
	}
	if cfg.Model == "" {
		return fmt.Errorf("no model selected. Pass --model <id>; `accred models` lists them")
	}
	reply, err := client.Chat(context.Background(), cfg.Model, []api.Message{{Role: "user", Content: prompt}}, cfg.MaxTokens, api.NewIdempotencyKey())
	if err != nil {
		return err
	}
	fmt.Println(reply.Content)
	fmt.Fprintf(os.Stderr, "%s · %d in / %d out · %s credits\n", reply.Model, reply.InputTokens, reply.OutputTokens, reply.CreditsCharged)
	return nil
}

func login() error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("login needs an interactive terminal")
	}
	fmt.Print("Paste your Accred API key (input is hidden): ")
	raw, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := verifyAndSaveKey(ctx, strings.TrimSpace(string(raw))); err != nil {
		return err
	}
	fmt.Println("Key verified and saved to your system keychain. Run `accred` to start.")
	return nil
}

// verifyAndSaveKey checks a key created on the Accred website and stores it in the keychain.
func verifyAndSaveKey(ctx context.Context, key string) error {
	if !strings.HasPrefix(key, "ct_live_") {
		return fmt.Errorf("that does not look like an Accred key; they start with ct_live_")
	}
	if err := api.New(key).ValidateKey(ctx); err != nil {
		return fmt.Errorf("the key was not accepted: %w", err)
	}
	if err := config.SaveAPIKey(key); err != nil {
		return fmt.Errorf("could not save to the system keychain: %w", err)
	}
	return nil
}

func listModels(filter string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	models, err := api.New("").Models(ctx)
	if err != nil {
		return err
	}
	terms := strings.Fields(strings.ToLower(filter))
	shown := 0
	for _, m := range models {
		haystack := strings.ToLower(m.ID + " " + m.Provider)
		match := true
		for _, t := range terms {
			match = match && strings.Contains(haystack, t)
		}
		if !match {
			continue
		}
		in, out := "-", "-"
		if m.InputCost != nil && m.OutputCost != nil {
			in, out = "$"+*m.InputCost, "$"+*m.OutputCost
		}
		fmt.Printf("%-52s %10s in %10s out  (per M tokens)\n", m.ID, in, out)
		shown++
	}
	fmt.Fprintf(os.Stderr, "%d models\n", shown)
	return nil
}
