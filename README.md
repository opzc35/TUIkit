# TUIkit

Toolkit of SSH Connection.

TUIkit is a Go SSH application built with `golang.org/x/crypto/ssh`. Users connect with an SSH client, then use an in-terminal TUI to register, log in, and access user or administrator menus.

## Features

- SSH server using `golang.org/x/crypto/ssh`
- Terminal UI over SSH
- User registration and login
- Password hashing with bcrypt
- Local JSON user database
- Automatic host key generation
- Administrator menu for user listing, role changes, activation, password reset, and deletion

## Run

```sh
go run ./cmd/tuikit -addr :2222
```

On first startup, TUIkit creates an `admin` account. To choose the first admin password:

```sh
TUIKIT_ADMIN_PASSWORD='change-this-password' go run ./cmd/tuikit -addr :2222
```

If `TUIKIT_ADMIN_PASSWORD` is not set, a random password is printed once in the server logs.

Then connect from another terminal:

```sh
ssh -p 2222 localhost
```

The default files are:

- `data/users.json` for users
- `data/host_key` for the SSH host key

## Build

```sh
go build ./...
```
