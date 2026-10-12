# Housenka

TUI aplikace v Go pro vykazování práce do Bílého motýla (BM). Uživatelská dokumentace je v [README.md](README.md).

## Prostředí: Claude běží v sandboxu

Claude Code tu běží v izolovaném kontejneru (konfigurace v `.hole/`), ne přímo na uživatelově WSL. To má tyto důsledky:

- **Síť je omezená allowlistem** (`.hole/settings.json`): povolené jsou jen `proxy.golang.org` a `sum.golang.org`. `go get` / `go mod tidy` přes Go proxy fungují. Ostatní síť (GitLab, BM, dokumentace přes web, `go.dev`) nemusí být dostupná, takže selhání sítě neznamená chybu v kódu.
- **Go**: setup skript instaluje Go 1.25.1 do `/usr/local/go`, `go.mod` vyžaduje `go 1.26.1` a toolchain se stáhne automaticky přes Go proxy (`GOTOOLCHAIN=auto`).
- **Nejsou tu prohlížeč ani schránka** (`wslview`, `xdg-open`, `cmd.exe`, `clip.exe`, `xclip`, `wl-copy` chybí). Otevírání URL a kopírování do schránky se tu nedá ověřit naživo; testovat přes abstrakci / fake příkaz. Na uživatelově stroji (WSL) budou dostupné.
- **Terminál** je bez TTY v interaktivním smyslu a `TERM=xterm`. TUI ověřovat testy v `internal/tui/app_test.go` (posílají klávesy do modelu). Obrazovky jako text: `HOUSENKA_SCREENS=<adresář> go test ./internal/tui -count=1`. Kouřový test skutečné binárky: `(sleep 1.5; printf q) | timeout 10 script -qfc "stty cols 110 rows 30; ./housenka" out.txt` a v `out.txt` hledat texty obrazovky.
- Pro ruční zkoušky používat `HOUSENKA_HOME=<scratchpad>/hh`, nikdy ne skutečné `~/.housenka`.
- Do `.hole/` nesahat, pokud o to uživatel výslovně nepožádá.

## Konvence

- **Repo je veřejné.** Do souborů v repu nikdy nepsat skutečné interní údaje: adresu BM, názvy polí formuláře BM, doménu firemního GitLabu, názvy klientských repozitářů a projektů, kódy činností z BM. V příkladech a testech používat `example.com`, `clients/shop-web`, `SHOPDEV` apod. Skutečné hodnoty patří jen do `config.private.yaml` (je v `.gitignore`) a do `~/.housenka/config.yaml`.
- Komunikace s uživatelem česky; kód, identifikátory a komentáře anglicky, texty v UI česky.
- Před dokončením: `gofmt -l .` (bez výstupu), `go vet ./...`, `go test ./...`.
- Čas se interně drží v čtvrthodinách (`int`), nikdy jako float.
