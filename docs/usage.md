# Using Share App

## Connect from a phone

Share App listens on `127.0.0.1:8443` by default, which is reachable only from the PC itself. It has no login or access control: use a trusted private network, restrict who can connect, and do not expose the host directly to the public internet.

For access over a trusted LAN:

1. Start Share App once to create its settings, then exit it from the notification icon.
2. Open `%LOCALAPPDATA%\ShareApp\config.json` and change `listenAddr` to `"0.0.0.0:8443"`, preserving the other settings.
3. Mark the Windows network as **Private**. In an administrator PowerShell, allow TCP 8443 from your LAN. Replace the example subnet with your actual network:

   ```powershell
   New-NetFirewallRule -DisplayName 'Share App TCP 8443' -Direction Inbound -Action Allow -Protocol TCP -LocalPort 8443 -RemoteAddress 192.168.3.0/24 -Profile Private
   ```

4. Restart Share App. On the phone, open `http://<PC-IP-address>:8443/` and select a window. Run `ipconfig` on the PC to find its LAN address.

A private network such as Tailscale can also provide connectivity. An authenticated HTTPS proxy can forward the web UI to `http://127.0.0.1:8443`, but WebRTC media still needs a reachable network path between the phone and PC. An HTTP proxy alone does not relay the video.

## Controls

Choose relative-pointer gestures or direct tapping to control the selected window. Use the text editor to send text and the special-key palette for keyboard commands.

The **Window / PC** button switches control modes without reconnecting. Each connection starts in **Window**, which uses the existing window-message input. **PC** includes menus and popups in the video and uses the PC’s real cursor and keyboard focus. Gesture settings, cursor position and the text draft are retained. Input pauses until the host confirms the first sample from the new capture.

When entering PC mode, other application windows are minimized and the selected window is restored and brought forward. Existing menus may close; menus opened afterward keep their focus, including when text is pasted into them. Minimized windows are left that way when you switch back or disconnect; use **Win+Shift+M** on the PC to restore them. Returning to Window mode brings the main window forward and may close popups.

Popups are clipped at the shared window’s boundary. PC control shares input with anyone using the PC, and cannot inject input into applications running as administrator. If another window covers a pointer press or scroll position, the input is rejected and a notice appears. A window can still move between this check and injection. Switching briefly pauses video; a few old frames may arrive after confirmation.

Sending text replaces the PC's clipboard and pastes it with Ctrl+V. Window mode brings the selected window to the foreground; PC mode retains focus on its popup if one is already active. Each send supports up to 16 KiB of text.

## Settings and logs

The gear button beside **Refresh** changes video settings and scroll sensitivity. They are shared by all devices and saved in `%LOCALAPPDATA%\ShareApp\config.json`:

```json
{
  "listenAddr": "127.0.0.1:8443",
  "captureStats": "off",
  "fps": 8,
  "crf": 31,
  "maxScale": 2,
  "scrollSensitivity": 1
}
```

| Setting | Purpose |
| --- | --- |
| `listenAddr` | HTTP listen address. Use `0.0.0.0:8443` to listen on network interfaces. HTTPS requires a separate proxy. |
| `captureStats` | Diagnostics: `off`, `on`, or `verify`. `verify` adds CPU-intensive pixel checks. |
| `fps` | Video frame rate, 1–30. Changes apply to the next connection. |
| `crf` | VP9 quality, 0–63. Lower values are sharper and use more bandwidth. Changes apply to the next connection. |
| `maxScale` | Window pixels per browser CSS pixel, limited to 0.5–4. Lower values make the shared window smaller and reduce video data. Changes apply to the next connection or viewport change. |
| `scrollSensitivity` | Touch scroll multiplier, 0.5–4 (default 1). Applies to both direct and cursor modes. At 1×, 100 CSS pixels of vertical finger travel equal one wheel notch; at 2×, 50 pixels do. Changes apply to the next connection. |

The browser saves these settings while preserving `listenAddr` and `captureStats`. To edit the file directly, exit Share App first and restart it afterward. All six fields are required; missing, null, unknown, or invalid fields prevent startup. To reset the settings, exit Share App and delete `config.json`; the next start creates a complete file with the defaults.

Right-click the notification icon and choose **Open data folder** to find settings and logs. The log file is `logs\share-host.log` in that folder. Installed, portable, and development builds use the same user data location.

## Upgrade and uninstall

Exit Share App before upgrading or uninstalling. Run the new Setup EXE to upgrade, or use **Settings → Apps → Installed apps** to uninstall.

The installer stores programs in `%LOCALAPPDATA%\Programs\ShareApp`. Settings, logs, and RDP credentials remain in the separate `%LOCALAPPDATA%\ShareApp` folder after uninstalling. Remove that folder manually to delete your data.

## Optional RDP keep-alive

A Windows session can lose its display when a Remote Desktop client disconnects, stopping capture. **Keep alive** holds a local RDP connection so that session retains a display.

1. Enable Remote Desktop on the PC.
2. As the Windows user who runs Share App, open PowerShell and generate credentials. For an installed copy:

   ```powershell
   powershell.exe -NoProfile -ExecutionPolicy Bypass -File "$env:LOCALAPPDATA\Programs\ShareApp\scripts\create-rdp-credentials.ps1"
   ```

   For the ZIP edition, use `-File .\scripts\create-rdp-credentials.ps1` from the extracted folder. Enter your Windows username and password, not a Windows Hello PIN.
3. Restart Share App, then press **Keep alive** in the window list.

The helper encrypts credentials using Windows DPAPI and writes `%LOCALAPPDATA%\ShareApp\rdp-credentials.bin`. Generate the file on the PC under the user who will run Share App. Credentials cannot be managed through the web UI.

Normal sharing works without this file. An invalid or undecryptable credential file prevents startup; check the log for details. To replace credentials, exit Share App and rerun the helper with `-Force`. To disable keep-alive, exit Share App and remove the file.

Keep-alive requests a 1920×1080 session display. If it disconnects, including when a person takes over the RDP session, press **Keep alive** again to reconnect.

## Troubleshooting

- **Windows version:** Windows 11 24H2 (build 26100) or later x64 is required. The installer and ZIP host reject older builds; the startup log includes the detected build.
- **The host will not start:** check `logs\share-host.log` for configuration, credential, or port errors.
- **The phone cannot open the page:** check the listen address, the PC's IP address, and the firewall rule's subnet and Private network profile.
- **The page opens but video will not connect:** the browser must support VP9 profile 0, and the devices must have a network path for WebRTC media as well as HTTP.
- **The picture stops updating:** restore minimized windows and move the shared window to the active virtual desktop. For a disconnected RDP session, use keep-alive.

If the problem persists, [open an issue](https://github.com/tanetakumi/remote-window-control/issues) with the steps to reproduce it and relevant log excerpts. Remove private information before sharing logs.
