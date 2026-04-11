# mal

## watch-order dataset

relations are loaded from a local dataset file instead of live scraping chiaki at request time.

### runtime

- env var: `WATCH_ORDER_FILE`
- default path: `./data/watch_order.json`

### regenerate dataset

1. refresh seed ids from local db

```sh
python3 - <<'PY'
import sqlite3, json, pathlib
conn = sqlite3.connect('mal.db')
cur = conn.cursor()
cur.execute('select id from anime order by id')
ids=[r[0] for r in cur.fetchall()]
path=pathlib.Path('tmp/watch_order_seed_ids.json')
path.write_text(json.dumps({'ids': ids}, indent=2), encoding='utf-8')
print(f'wrote {len(ids)} ids to {path}')
PY
```

2. generate dataset json

```sh
go run ./cmd/watchorder -seed "tmp/watch_order_seed_ids.json" -out "data/watch_order.json"
```

3. restart the server/container to load the updated file
