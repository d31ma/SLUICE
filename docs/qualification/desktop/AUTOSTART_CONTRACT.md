# Start-at-Login Contract

## Public behavior

`internal/platform/autostart.Manager` exposes `Enable`, `Disable`, and
`Status`. Status is one of:

- `disabled`: no Sluice registration exists;
- `enabled`: the registration points to the exact canonical executable and
  includes only the fixed `--background` argument; or
- `needs-repair`: Sluice's registration exists but differs from the expected
  executable or format.

Calling enable repairs a stale registration atomically. Calling disable is
idempotent. A canceled context fails before mutation. Once a small local
mutation begins, the operation finishes rather than returning an ambiguous
cancellation error after changing state.

The executable must resolve to an absolute, existing regular file. On macOS
and Linux it must have an executable permission bit. Symbolic links are
resolved before registration, so an upgrade is visible as `needs-repair`
until enable writes the new installed path.

## Platform registrations

### Windows

Sluice writes the `Sluice` string value under the current user's:

```text
HKCU\Software\Microsoft\Windows\CurrentVersion\Run
```

The value is a Windows-command-line-safe, quoted executable path followed by
the literal `--background` argument. No administrator access, scheduled task,
service, shell, or system-wide registry key is used.

### macOS

Sluice writes this per-user LaunchAgent:

```text
~/Library/LaunchAgents/ma.del.sluice.desktop.plist
```

The property list uses `ProgramArguments`, not a shell command. It specifies
`RunAtLoad` and the Aqua session type. Enable and disable affect subsequent
login sessions; Sluice does not invoke `launchctl` or launch a second copy in
the current session.

### Linux

Sluice writes this XDG autostart entry:

```text
${XDG_CONFIG_HOME:-~/.config}/autostart/ma.del.sluice.desktop.desktop
```

`XDG_CONFIG_HOME`, when set, must be absolute. Reserved desktop-entry command
characters and percent field codes are escaped. The entry is non-terminal and
explicitly enabled for desktop environments that honor the GNOME extension.
Desktop environments without XDG autostart support are not qualified by this
adapter.

## File safety

macOS and Linux registrations are written through a same-directory temporary
file, flushed, renamed, and followed by a directory flush. New registration
directories use mode `0700`; registration files use mode `0600`. Sluice will
not read, replace, or delete a registration path that is a symbolic link or a
non-regular file.

These protections cover the final Sluice-owned registration path. They do not
replace normal OS account security, installer signature validation, or native
upgrade testing.

## Native acceptance procedure

Use an isolated test account or disposable VM and an installed artifact.

1. Confirm `autostart status` prints `disabled`.
2. Run `autostart enable` twice and confirm both calls succeed.
3. Confirm status prints `enabled` and inspect the native registration.
4. Replace the registered executable path with a harmless stale value and
   confirm status prints `needs-repair`.
5. Run enable and confirm status returns to `enabled`.
6. Log out and back in. Confirm one Sluice process starts with its window
   hidden and a working tray entry.
7. Run disable twice, log out and back in, and confirm Sluice does not start.

Windows registry execution, macOS real-login loading, and Linux desktop-login
loading must be recorded independently before their corresponding native gate
passes.
