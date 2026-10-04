<p align="center">
  <img src="https://raw.githubusercontent.com/useAccred/accred-cli/main/assets/mascot.png" alt="The Accred fox" width="200">
</p>

<h1 align="center">Accred CLI</h1>

<p align="center">
  Chat with any AI model from your terminal and pay with your <a href="https://accred.sh">Accred</a> credits.<br>
  One API key, one balance, and the exact cost shown under every reply.
</p>

---

## Contents

- [What it is](#what-it-is)
- [Quick start](#quick-start)
- [Getting an API key](#getting-an-api-key)
- [Using the chat terminal](#using-the-chat-terminal)
- [Using it in scripts](#using-it-in-scripts)
- [What a reply costs](#what-a-reply-costs)
- [Where your key is stored](#where-your-key-is-stored)
- [Troubleshooting](#troubleshooting)
- [What it does not do yet](#what-it-does-not-do-yet)
- [Updating and uninstalling](#updating-and-uninstalling)
- [Build from source](#build-from-source)

## What it is

Accred turns LLM usage into credits you hold in your own wallet: 100 credits equal $1 of model usage, with no markup on model prices. This CLI lets you spend those credits from a terminal:

- **Hundreds of models, one key.** Pick any text model in the Accred catalog and switch at any time.
- **Costs you can see.** Every reply shows the tokens used and the exact credits charged. The status bar shows your remaining balance.
- **Never charged twice.** If your connection drops, the call is retried safely.
- **A fox.** The Accred mascot keeps you company above the input box.

## Quick start

You need [Node.js](https://nodejs.org) 18 or newer.

```bash
npm install -g @useaccred/cli
accred
```

Or run it once without installing:

```bash
npx @useaccred/cli
```

The first time you start it:

1. It asks for your API key. Paste it and press enter. ([How to get one](#getting-an-api-key))
2. It asks you to pick a model. Type to filter, use the arrow keys, press enter.
3. Type a message and press enter.

That's it. Your key and model are remembered for next time.

> The npm package is a small launcher. On first run it downloads the program for your computer from this project's [GitHub releases](https://github.com/useAccred/accred-cli/releases), checks it against a SHA-256 checksum shipped in the package, and keeps it in `~/.accred/bin`.

**Supported systems:** macOS (Apple silicon and Intel), Linux (x64 and arm64), Windows (x64). Development and testing happen on macOS. The Linux and Windows builds have had no hands-on testing yet, so please [report](https://github.com/useAccred/accred-cli/issues) anything that misbehaves.

## Getting an API key

1. Open [accred.sh](https://accred.sh), sign in, and verify your wallet.
2. Get credit on the **Buy** or **Swap** page.
3. On the **Wallet** page, **activate** some credit. The CLI spends activated credit only; credit sitting in your wallet is not used.
4. On the **API** page, create a key and copy it. It starts with `ct_live_` and is shown only once.

Treat the key like a password: anyone who has it can spend your credit. You can revoke it on the API page at any time.

## Using the chat terminal

Start it with `accred`. Type a message and press enter. Type `/` to see the commands.

### Commands

| Command | What it does |
| --- | --- |
| `/model` | Open the model list. `/model gpt` opens it filtered. |
| `/max 2000` | Set the most tokens a reply may use (1 to 8192). Lower means less credit held per call. |
| `/system You are brief.` | Set a system prompt. `/system` alone clears it. |
| `/cost` | Show credits spent in this session and your available balance. |
| `/clear` | Start a new conversation. |
| `/mascot off` | Hide the fox. `/mascot on` brings it back. |
| `/login` | Add or replace your API key. |
| `/logout` | Forget the saved API key. |
| `/key` | Show where the current key comes from. |
| `/help` | List commands and keys. |
| `/quit` | Exit. |

### Keys

| Key | Action |
| --- | --- |
| enter | Send the message |
| alt+enter or ctrl+j | Start a new line |
| ↑ and ↓ | Bring back messages you sent earlier |
| tab | Complete a command |
| esc | Cancel the request in progress |
| ctrl+c twice | Exit |

### The fox

The fox walks and wags its tail while you type, hurries while a model is working, hops when things go well, and falls asleep after a minute of quiet. It needs a window at least 20 rows tall and hides on smaller ones.

It is a small drawing of the mascot at the top of this page, made of coloured terminal characters. Terminals show characters, not pictures, so it is not the image itself.

## Using it in scripts

```bash
# One reply, then exit. The reply goes to standard output, the cost to standard error.
accred --model MODEL_ID -p "Explain DNS in one paragraph"

# Read the prompt from a pipe
cat notes.txt | accred --model MODEL_ID -p "Summarize this:"

# List available models and prices; add a word to filter
accred models
accred models mistral
```

Replace `MODEL_ID` with an id from `accred models`. If you have already picked a model in the chat terminal, you can leave `--model` out.

For scripts and servers, set the key in the environment instead of the keychain:

```bash
export ACCRED_API_KEY=ct_live_...
```

| Flag | Meaning |
| --- | --- |
| `-p "text"` | Prompt for a single reply |
| `--model ID` | Model to use for this run |
| `--max-tokens N` | Most tokens the reply may use, 1 to 8192 |
| `--no-mascot` | Hide the fox |
| `--version` | Print the version |

## What a reply costs

- You pay the model's own price. The model list shows it per million tokens, for input and output.
- Before a call, credit for the worst case (your messages plus the token limit) is put on hold. After the reply, the real cost is charged and the rest is released at once.
- If a call fails before the model answers, nothing is charged.
- Each call carries an idempotency key that is reused on retries, so a dropped connection never results in a double charge.

To spend less: use a smaller model for simple tasks, lower the limit with `/max`, and use `/clear` when you change topic, because every message sends the whole conversation again.

## Where your key is stored

- **API key:** in your system keychain (Keychain on macOS, Credential Manager on Windows, Secret Service on Linux). It is never written to a file by the CLI.
- **Preferences** (model, token limit, fox on or off): `~/.config/accred/config.json`.
- **The program itself:** `~/.accred/bin`.

If `ACCRED_API_KEY` is set, it takes precedence over the keychain. On Linux without GNOME Keyring or KWallet, use `ACCRED_API_KEY`.

## Troubleshooting

**"Not enough credit" or a 402 error.**
The CLI spends activated credit. Open the Wallet page at accred.sh and activate more, or lower the limit with `/max`.

**"The platform API key is invalid or revoked."**
Run `/login` (or `accred login`) and paste a working key from the API page.

**`npm install` says "404 Not Found" for a different package.**
You ran a plain `npm install` inside a folder that has its own `package.json` with a dependency that no longer exists. Install globally instead: `npm install -g @useaccred/cli`.

**`npx @useaccred/cli` keeps running an old version.**
`npx` reuses the copy it already downloaded. Run `npm install -g @useaccred/cli` to get the current one.

**The fox does not appear.**
The window is shorter than 20 rows, or it was turned off. Make the window taller, or run `/mascot on`.

**The fox has thin lines across it.**
Some terminals, including the built-in macOS Terminal, draw block characters slightly short of the cell edge. iTerm2, Ghostty, kitty, WezTerm, Windows Terminal and the VS Code terminal draw it cleanly.

**A long pause before the reply appears.**
See the next section: replies arrive in one piece.

**The download fails or the checksum does not match.**
Check your connection and run the command again. If it keeps failing, download the binary for your system from the [releases page](https://github.com/useAccred/accred-cli/releases) and run it directly.

## What it does not do yet

- **Live streaming.** The Accred API returns a reply in one piece after the model finishes, so the terminal waits and then types the reply out.
- **Account and wallet actions.** Creating keys, viewing usage history, buying, swapping, staking and transfers happen in the [Accred app](https://accred.sh).
- **Balance before the first reply.** The API reports your available credit with each reply, so the status bar shows it after you send a message.
- **Files, images and tools.** Chat is text only.
- **Saved conversations.** A conversation lasts until you exit or run `/clear`.

## Updating and uninstalling

```bash
npm install -g @useaccred/cli     # update to the newest version
npm uninstall -g @useaccred/cli   # remove the launcher
rm -rf ~/.accred ~/.config/accred # remove the downloaded program and preferences
```

Run `accred logout` before uninstalling to remove the key from your keychain.

## Build from source

Requires Go 1.27 or newer.

```bash
git clone https://github.com/useAccred/accred-cli
cd accred-cli
go build -o accred .
go test ./...
```

`scripts/build-release.sh` builds the release binaries and their checksums.

## Links

- [Accred](https://accred.sh) · [Documentation](https://accred.sh/docs) · [Report an issue](https://github.com/useAccred/accred-cli/issues)
- JavaScript SDK: [`accred`](https://www.npmjs.com/package/accred) · Vercel AI SDK provider: [`@useaccred/ai-sdk-provider`](https://www.npmjs.com/package/@useaccred/ai-sdk-provider)

## License

MIT
