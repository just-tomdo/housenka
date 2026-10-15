# Ukázková data

Každý podadresář je samostatný domov Housenky (`config.yaml` + `entries/`), takže se dá spustit bez zásahu do `~/.housenka`:

```sh
HOUSENKA_HOME=examples/demo housenka
```

- `demo/` – veřejné demo se smyšlenými projekty (e-shop, Acme, Ferda Mravenec…).
- `firma/` – volitelná soukromá ukázka nad skutečným configem; je v `.gitignore` a do repa nepatří.

## Generování dat

Záznamy v `entries/` vyrábí generátor podle `scenario.yaml` (issue, pravidelné porady, občasná školení…). Generuje pracovní dny od začátku měsíce o `months` (výchozí 6) dřív až do dneška: bez víkendů, českých svátků a pár dní dovolené, poslední tři dny jsou neodeslané a dnešek rozpracovaný. Výsledek je pro stejný scénář a den vždy stejný.

```sh
go run ./examples/gen examples/demo                 # data do dneška
go run ./examples/gen -to 2026-10-15 examples/demo  # data do zadaného dne
```

Existující `entries/*.json` v adresáři se přepíšou. Před prezentací stačí generátor pustit znovu, aby data končila dnešním dnem.
