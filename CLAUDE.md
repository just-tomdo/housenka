# Housenka

Aplikace v Go pro vykazování práce do Bílého motýla (BM) s TUI a webovým rozhraním (GUI). Uživatelská dokumentace je v [README.md](README.md).

## Rozhraní: TUI a GUI

- **TUI** (`internal/tui`, Bubble Tea) je cesta pro terminálové a vim nadšence. **GUI** (`internal/web`, `housenka web`: lokální server, `html/template` + HTMX) je uživatelsky přívětivá cesta pro ostatní.
- Mezi nimi platí **parita funkcí**: co umí TUI, musí umět i GUI (včetně klávesových zkratek). Novou funkci přidávej do obou rozhraní.
- Sdílená logika patří do doménových balíčků (`rules`, `stats`, `period`, `civil`, `hours`, `bm`, `store`) nebo do `internal/entryedit` (parsování formuláře, nabídky, ukládání, kopie, filtr, BM odkaz), nikdy do `internal/tui` ani `internal/web`.
- Výjimka parity: GUI má editor konfigurace (Nastavení: průvodce, repozitáře, projekty, činnosti, obecné, YAML; `internal/web/settings.go`). TUI ho mít nebude, tam zůstává ruční úprava YAML (`housenka config edit`).
- Změny configu dělá `internal/configedit`: upravuje strom `yaml.Node`, takže zůstávají komentáře a pořadí klíčů, a `pretty.go` vrací rozložení (prázdné řádky mezi sekcemi, sloupce komentářů, tabulky činností a repozitářů). Nezměněný soubor musí po uložení zůstat bajt po bajtu stejný a bloky, které se nezměnily, si drží přesné původní rozložení (prázdné řádky, zarovnání, CRLF; testy na `config.example.yaml`, `examples/demo` a ručně formátovaný config). Operace drží vazby (přejmenování a mazání kódů v `projects` a `repos`); každé uložení validuje celý config, kontroluje hash souboru (souběžná změna) a nechá zálohu `.bak`.
- `housenka web` startuje i bez configu (vytvoří ho ze vzoru, průvodce porovnává se vzorem) a s rozbitým configem (pak jen záložka YAML).
- TUI i GUI můžou běžet zároveň nad stejnými daty: store znovu načítá změněné soubory (`atomicfile.Stamp`: čas, velikost, inode) a zapisuje pod zámkem `entries/.lock`; web server znovu načte změněný config. `Store.Save` odmítne (`store.ErrConflict`) přepsat záznam, který se od načtení volajícím změnil – při ukládání vždy předávej záznam s jeho `UpdatedAt` (TUI z cache, web ze skrytého pole `version`).
- Soubory (záznamy i config) se zapisují jen přes `internal/atomicfile.Write`: atomicky, se zachováním symlinku a práv, na Windows s opakováním přejmenování.
- Drobné sdílené texty (`text.Plural`, `text.OneLine`), popisky období (`period.Kind.CurrentLabel`) a pravidla dnů (`civil.Date.IsWeekend`, `stats.Summary.WeekStrip`) nekopírovat do TUI ani webu.
- Server na pozadí (`internal/web/run.go`): stav v `~/.housenka/web.json` (PID, port, token), `/_/health` vrací `housenka <PID>`. Stavový soubor se maže, jen když proces neexistuje nebo je port volný; běžící server si ho každých 5 s obnoví. `web stop` ukončí proces podle PID, jen když ho `/_/health` potvrdí (nebo s `--force` u serveru, který neodpovídá). Testy s podprocesy v `run_test.go` (`TestMain`).
- Témata GUI jsou CSS soubory v `internal/web/static/themes/` (hlavička `/* name: …; scheme: light|dark */` a `:root` s pevnou sadou proměnných, viz `themeVars` v `internal/web/themes.go`). Nové téma = nový soubor.
- HTMX je embednutý v `internal/web/static/htmx.min.js` (verze v commitu, stažený přes Go proxy: `proxy.golang.org/github.com/bigskysoftware/htmx/@v/<verze>+incompatible.zip`).
- Bezpečnost serveru: poslouchá jen na `127.0.0.1`, kontroluje `Host` (jen `127.0.0.1`, `localhost` a `web.hostnames` načtené při startu; na portu 80 i bez portu), změnové požadavky vyžadují `Origin` stránky (chybějící `Origin` se odmítá) a CSRF token (cookie + hlavička `X-CSRF-Token`). Tohle neoslabovat.

## Prostředí: Claude běží v sandboxu

Claude Code tu běží v izolovaném kontejneru (konfigurace v `.hole/`), ne přímo na uživatelově WSL. To má tyto důsledky:

- **Síť je omezená allowlistem** (`.hole/settings.json`): povolené jsou jen `proxy.golang.org` a `sum.golang.org`. `go get` / `go mod tidy` přes Go proxy fungují. Ostatní síť (GitLab, BM, dokumentace přes web, `go.dev`) nemusí být dostupná, takže selhání sítě neznamená chybu v kódu.
- **Go**: setup skript instaluje Go 1.25.1 do `/usr/local/go`, `go.mod` vyžaduje `go 1.26.1` a toolchain se stáhne automaticky přes Go proxy (`GOTOOLCHAIN=auto`).
- **Nejsou tu nástroje pro otevření prohlížeče ani schránka** (`wslview`, `xdg-open`, `cmd.exe`, `clip.exe`, `xclip`, `wl-copy` chybí). Otevírání URL a kopírování do schránky se tu nedá ověřit naživo; testovat přes abstrakci / fake příkaz. Na uživatelově stroji (WSL) budou dostupné.
- **GUI** ověřovat testy v `internal/web/web_test.go` a `settings_test.go` (`httptest`). Kouřový test: `HOUSENKA_HOME=<scratchpad>/hh ./housenka web start --no-open`, pak `curl -H 'Host: housenka.localhost:8765' http://127.0.0.1:8765/` a `./housenka web stop`. Na vzhled a JS jde použít headless prohlížeč přes `agent-browser`, pokud je k dispozici.
- **Terminál** je bez TTY v interaktivním smyslu a `TERM=xterm`. TUI ověřovat testy v `internal/tui/app_test.go` (posílají klávesy do modelu). Obrazovky jako text: `HOUSENKA_SCREENS=<adresář> go test ./internal/tui -count=1`. Kouřový test skutečné binárky: `(sleep 1.5; printf q) | timeout 10 script -qfc "stty cols 110 rows 30; ./housenka" out.txt` a v `out.txt` hledat texty obrazovky.
- Pro ruční zkoušky používat `HOUSENKA_HOME=<scratchpad>/hh`, nikdy ne skutečné `~/.housenka`.
- Do `.hole/` nesahat, pokud o to uživatel výslovně nepožádá.

## Konvence

- **Repo je veřejné.** Do souborů v repu nikdy nepsat skutečné interní údaje: adresu BM, názvy polí formuláře BM, doménu firemního GitLabu, názvy klientských repozitářů a projektů, kódy činností z BM. V příkladech a testech používat `example.com`, `clients/shop-web`, `SHOPDEV` apod. Skutečné hodnoty patří jen do `config.private.yaml` (je v `.gitignore`) a do `~/.housenka/config.yaml`.
- Komunikace s uživatelem česky; kód, identifikátory a komentáře anglicky, texty v UI česky.
- Před dokončením: `gofmt -l .` (bez výstupu), `go vet ./...`, `go test ./...`.
- Čas se interně drží v čtvrthodinách (`int`), nikdy jako float.

## Vydání nové verze

Vyvíjí se na `development`, vydává se z `main`. Po mergi se na merge commit v `main` dá tag a pushne:

```sh
git checkout main && git pull
git tag v0.1.0
git push origin v0.1.0
```

Push tagu `v*` spustí `.github/workflows/release.yml`: kontroly a testy, sestavení binárek pro Linux, macOS a Windows (`scripts/build-release.sh`) a založení GitHub release s `checksums.txt`. Z posledního release instaluje `install.sh`. Verze se do binárky dostane z tagu (`housenka version`).

Lokálně se binárky sestaví do `dist/` (je v `.gitignore`) přes `scripts/build-release.sh v0.1.0`.
