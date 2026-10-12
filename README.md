# 🐛 Housenka

Terminálová aplikace pro vykazování práce do Bílého motýla (BM), která hlídá, aby kombinace issue, kódu projektu, činnosti a času dávala smysl.

- Z odkazu na GitLab issue pozná repozitář a nabídne **jen kódy projektů povolené v configu**.
- U projektu nabídne **jen povolené činnosti** a předvybere výchozí.
- Čas přijímá jen po čtvrthodinách (`2,75`, `2.5`, `1:45`).
- Pro každý záznam vygeneruje odkaz, který otevře BM s předvyplněným formulářem.
- Statistiky za den, týden a měsíc, poměr produktivní / neproduktivní práce a týdenní souhrn pro teamový status.

## Instalace

```sh
go install github.com/just-tomdo/housenka@latest
# nebo z repozitáře:
go build -o housenka .
```

## První spuštění

```sh
housenka config init      # vytvoří ~/.housenka/config.yaml se vzorem a komentáři
$EDITOR ~/.housenka/config.yaml
housenka config check     # ověří config, vypíše všechny chyby najednou
housenka parse https://gitlab.example.com/clients/acme-web/-/work_items/636   # zkouška mapování
housenka                  # spustí TUI
```

Ve vzorovém configu je potřeba vyplnit hlavně `bm.base_url` (adresa editoru výkazu) a `bm.fields` (názvy polí formuláře, které se předvyplní – zjistíš je z odkazu, který editor sám vytvoří), dále kódy činností a projektů a mapování repozitářů.

Vše je v adresáři `~/.housenka/` (config a `entries/<rok>.json`). Jiné umístění nastavíš přes `HOUSENKA_HOME`, jiný config přes `--config`.

## Ovládání

Klávesy jsou vim-friendly, ale každá akce má i běžnou alternativu. `?` zobrazí kompletní nápovědu.

| Akce | Klávesy |
|---|---|
| pohyb | `j`/`k`, `gg`/`G`, `ctrl+d`/`ctrl+u` · šipky, Home/End, PgDn/PgUp |
| předchozí / další den, týden či měsíc | `h`/`l` · ←/→ |
| dnes | `t` |
| den / týden / měsíc | `1`/`2`/`3` · Tab |
| přidat / upravit | `a` (`o`, Insert) / `e` (Enter) |
| kopie záznamu na dnešek | `c` |
| zapamatovat / vložit kopii na zobrazený den | `yy` / `p` |
| smazat | `dd` · Delete |
| otevřít v BM / kopírovat odkaz do BM | `b` / `Y` |
| přepnout „odesláno“ | `s` · Space |
| týdenní souhrn pro status | `S` (v souhrnu `y` zkopíruje text, `m` Markdown) |
| filtr | `/` (Esc zruší) |
| konec | `q`, `:q`, `ctrl+c` |

Ve formuláři: Tab / Enter další pole, Shift+Tab předchozí pole, `h`/`l` nebo `j`/`k` vybírají kód a činnost. Popis je víceřádkový (Enter = nový řádek, Tab = dál). Na konci jsou tlačítka **Uložit** a **Zrušit** (Enter je stiskne); zkratky `ctrl+s` uloží a Esc zruší odkudkoli. Datum přijme `12. 10. 2026`, `12.10.`, `dnes`, `včera` nebo `-1`.

Úprava už odeslaného záznamu ho označí jako neodeslaný – v BM je pak potřeba upravit i původní výkaz.

## Podporované systémy

Linux, macOS, Windows a WSL. Prohlížeč a schránka se detekují automaticky (`xdg-open`, `open`, `wslview`, `rundll32`; `wl-copy`, `xclip`, `pbcopy`, `clip.exe`). Když schránkový nástroj chybí, kopíruje se přes terminál (OSC 52). Příkaz prohlížeče jde přepsat v `bm.open_command`.
