# apollo2punkt

[Español](README.es.md)

Imports the games from a Vibepollo, Apollo or Sunshine `apps.json` into the
[Punktfunk](https://github.com/punktfunk/punktfunk) library on the same Windows PC. Each game's
launch command is copied as it is, so games launched through scripts, PsExec, Steam or their
own `.exe` keep launching the same way.

The program speaks Spanish.

## Use

1. Download `apollo2punkt.exe` from [Releases](../../releases).
2. Make sure the Punktfunk host is running on the PC.
3. Right-click `apollo2punkt.exe` → **Run as administrator**. It needs this to read the host's
   management token in `%ProgramData%\punktfunk\mgmt-token`.
4. It finds the `apps.json` by itself, lists what it will import and asks which games to
   import (`1,3-5`, or Enter for all).

The `.exe` is not signed: Windows SmartScreen may warn the first time. Choose **More info →
Run anyway**, or build it yourself (below).

### Options

From an administrator terminal:

| Option | What it does |
| --- | --- |
| `-apps PATH` | use this `apps.json` |
| `-export test.json` | doesn't touch Punktfunk: saves what it would send, to review it |
| `-update` | overwrites games it already imported |
| `-undo` | removes from Punktfunk everything this program imported |
| `-steam` | also imports Steam games the Steam plugin already lists |
| `-no-art` | doesn't look for covers |
| `-yes` | everything without asking |

It writes `apollo2punkt-state.json` beside the `.exe`. Keep it for `-update` and `-undo`.

## What it does with each app

- **Where it looks for `apps.json`:** beside the `.exe`, then
  `Program Files\{Vibepollo,Apollo,Sunshine}\config\`, then `X:\{Vibepollo,Apollo,Sunshine}\config\`
  on every drive.
- **Launch:** the command, detached commands, working directory and prep/undo commands are kept.
- **Detection**, so Punktfunk knows when the game closes:
  - `steam://rungameid/N` → the Steam game N;
  - an `.exe` in the command → that exe and its folder. Wrappers (PowerShell, PsExec, `cmd`,
    `steam.exe`…) are skipped, so a `launch.ps1 "D:\Game\game.exe"` watches `game.exe`;
  - a `.bat` → its folder;
  - nothing found → imported anyway, with a warning.
- **Steam duplicates:** a Steam game the Steam plugin already lists (`steam:N` in the library) is
  skipped. Use `-steam` to import it anyway.
- **Cover:** when Vibepollo/Apollo saved it as `igdb_<id>.png`, the same IGDB cover and artwork
  are used. Other local images are not copied: pick one later in Punktfunk.
- **Skipped:** Desktop, Remote Input and Remote Monitor (they launch nothing; Punktfunk has its own
  Desktop) and Steam Big Picture (the Steam plugin adds it).
- Vibepollo-only options (Lossless Scaling, `config-overrides`…) have no Punktfunk equivalent;
  it warns about the ones you had on.

## Build

With [Go](https://go.dev):

```sh
go test ./...
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o apollo2punkt.exe .
```

## License

MIT or Apache-2.0, at your option.
