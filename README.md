# JGRPP Launcher

A desktop launcher for [JGR's Patchpack](https://github.com/JGRennison/OpenTTD-patches) (JGRPP) for OpenTTD. Handles profile management, version downloads, and multiplayer configuration.

## Features

- **Profile system** — Set up separate profiles for different saves, servers, or JGRPP versions. Switch between them with a click.
- **Auto-download** — If the JGRPP version you need isn't installed locally, the launcher grabs it from GitHub and extracts it for you.
- **Version pinning** — Lock a profile to a specific JGRPP version (e.g. `0.71.1`) or just use `latest` to always grab the newest release.
- **Multiplayer quick-join** — Store server address, passwords, and company info per profile. The launcher passes the right arguments so you connect directly.
- **Save path loading** — Point a profile at a save folder and the launcher will load the most recent `.sav` file automatically.

## Getting Started

1. **Download** the latest release for your OS from the [Releases](../../releases) page.
2. **Run the executable** — no installer needed.
   - **Windows**: `JGRPP_Launcher.exe`
   - **Linux**: `JGRPP_Launcher_linux` (you may need to `chmod +x` it first)
   - **macOS**: `JGRPP_Launcher_macos` (right-click → Open to bypass Gatekeeper on first run)
3. On first launch you'll be asked to confirm your install paths. The defaults are usually fine.
4. Create a profile, pick a version, and hit **Run Selected**.

## Profiles

Each profile stores:

| Field | What it does |
|---|---|
| **Name** | Display name in the list |
| **Version** | JGRPP version to use — `latest` or a specific tag like `0.72.2` |
| **Save Path** | Relative path under your OpenTTD `save/` folder (e.g. `My Games/Coastal`). Loads the most recent `.sav` in that folder. Can also be an absolute path or left blank. |
| **Server IP:Port** | Multiplayer server address, e.g. `play.example.com:3979` |
| **Server Password** | Server join password (if any) |
| **Company Number** | Which company slot to join |
| **Company Password** | Company password (if any) |

You can **create**, **edit**, **duplicate**, and **delete** profiles from the main window. Profiles are stored in `config.json` next to the executable.

## Settings

Click **Settings** at the bottom of the profile list to configure:

- **Parent Directory** — Where JGRPP versions are downloaded and extracted to.
- **Docs Base Path** — Your OpenTTD documents folder (where `save/` and `openttd.cfg` live).
- **Auto-close** — Optionally close the launcher once OpenTTD starts.
- **Verbose logging** — Show detailed log messages during launch.
- **GitHub API URL** / **OS Type** — Advanced settings, they're auto-detected..

## Config File

The launcher stores all settings and profiles in a single `config.json` file. It looks for this file next to the executable. You can edit it by hand if you prefer, but the UI covers everything.

## Troubleshooting

- **Launcher exits immediately** — Enable `logToFile` manually in `config.json` and check `log.txt` in the same folder.
- **"Executable not found"** — Make sure your Parent Directory path is correct and contains extracted JGRPP folders.
- **Save not loading** — Check that the Save Path in your profile matches an actual folder under your OpenTTD `save/` directory. The launcher picks the newest `.sav` file in that folder.
- **Download fails** — The launcher needs internet access to reach the API. Check your connection and firewall settings.
- **macOS: "app is damaged"** — This is Gatekeeper blocking an unsigned binary. Right-click the file → Open → Open to allow it.

## Building from Source

Requires Go 1.26+. Linux builds also need `libgl1-mesa-dev` and `xorg-dev`.

```bash
# Windows
go build -ldflags "-H=windowsgui" -o dist/JGRPP_Launcher.exe .

# Linux / macOS
go build -o dist/JGRPP_Launcher .
```

## License

This project is licensed under the [Creative Commons Attribution-NonCommercial-ShareAlike 4.0 International License](LICENSE) (CC BY-NC-SA 4.0).
