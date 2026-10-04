<p align="center">
  <img src="https://raw.githubusercontent.com/useAccred/accred-cli/main/assets/mascot.png" alt="The Accred fox" width="220">
</p>

<h1 align="center">Accred CLI</h1>

<p align="center">
  <a href="https://accred.sh">Accred</a> in your terminal. Chat with any model in the catalog and pay from your tokenized LLM credits, with the exact cost shown under every reply.
</p>

## Install

```bash
npm install -g @useaccred/cli
accred
```

or run it without installing:

```bash
npx @useaccred/cli
```

The npm package is a small launcher. On first run it downloads the native program for your machine from this repository's [releases](https://github.com/useAccred/accred-cli/releases), checks it against a SHA-256 checksum shipped in the package, and keeps it in `~/.accred/bin`. Requires Node 18 or newer.

Builds are published for macOS (Apple silicon and Intel), Linux (x64 and arm64) and Windows (x64). Development and testing happen on macOS; the Linux and Windows builds are cross-compiled and have had no hands-on testing yet, so please [report](https://github.com/useAccred/accred-cli/issues) anything that misbehaves.

## Set up

1. Sign in at [accred.sh](https://accred.sh) and verify your wallet.
2. Buy or swap into credit, then activate it on the Wallet page. The CLI spends activated credit.
3. Create a key on the API page.
4. Run `accred`. The first time, it asks you to paste the key, checks it, and saves it in your system keychain.

You can also set the key for the current shell instead:

```bash
export ACCRED_API_KEY=ct_live_...
```

## Use

```bash
accred                       # open the chat terminal
accred -p "Explain DNS"      # print one reply and exit
cat notes.txt | accred -p "Summarize this"
accred models mistral        # list models and prices, filtered
```

On first run the terminal asks you to pick a model. The choice is remembered.

### Commands

| Command | What it does |
| --- | --- |
| `/model [name]` | Choose a model, or filter the list by name |
| `/max <tokens>` | Set the most tokens a reply may use (1 to 8192) |
| `/system [text]` | Set a system prompt; no text clears it |
| `/cost` | Credits spent in this session |
| `/clear` | Start a new conversation |
| `/mascot on\|off` | Show or hide the fox |
| `/login` | Add or replace your API key |
| `/logout` | Forget the saved API key |
| `/key` | Where the API key comes from |
| `/help` | List commands |
| `/quit` | Exit |

### Keys

| Key | Action |
| --- | --- |
| enter | Send |
| alt+enter, ctrl+j | New line |
| ↑ ↓ | Previous and next messages you sent |
| tab | Complete a command |
| esc | Cancel the request in progress |
| ctrl+c twice | Exit |

## The fox

The fox above the input box is the Accred mascot, redrawn by hand as a small sprite in four flat colours so it stays crisp in a terminal. It wanders and wags its tail while you type, hurries while a model is working, hops when things go well, and dozes off after a minute of quiet. It hides itself on screens shorter than 20 rows, and `/mascot off` hides it for good.

Terminals draw characters, not pictures, so this is a drawing of the mascot at the top of this page, not the image itself.

## What it does not do yet

- **Live streaming.** The Accred API returns a reply in one piece after the model finishes, so the terminal waits and then types the reply out.
- **Account and wallet actions.** Creating keys, checking usage history, buying, swapping, staking and transfers happen in the [Accred app](https://accred.sh). The CLI only needs an API key.
- **Balance before the first reply.** The API reports your available credit with each reply, so the status bar shows it after you send a message.
- **Files, images and tools.** Chat is text only.

## How billing works

Credit for the worst case of a call (your messages plus the token limit) is held up front; the real cost is charged when the reply arrives and the rest is released. Each call carries an idempotency key that is reused on retries, so a dropped connection never results in a double charge.

## Files

Preferences are stored in `~/.config/accred/config.json` (or under `XDG_CONFIG_HOME`). The API key is stored in the system keychain, never in that file. On Linux the keychain needs a running Secret Service (GNOME Keyring or KWallet); without one, use `ACCRED_API_KEY`.

## Build from source

Requires Go 1.27 or newer.

```bash
git clone https://github.com/useAccred/accred-cli
cd accred-cli
go build -o accred .
go test ./...
```

`scripts/build-release.sh` builds the release binaries and their checksums.

## License

MIT
