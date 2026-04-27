#!/usr/bin/env bash

set -e

usage() {
  echo "Brug: $0 --username=<brugernavn> --password=<adgangskode>"
  exit 1
}

USERNAME=""
PASSWORD=""

for i in "$@"; do
  case $i in
    --username=*)
      USERNAME="${i#*=}"
      shift
      ;;
    --password=*)
      PASSWORD="${i#*=}"
      shift
      ;;
    *)
      echo "Ukendt parameter: $i"
      usage
      ;;
  esac
done

if [ -z "$USERNAME" ] || [ -z "$PASSWORD" ]; then
  usage
fi

export USERNAME
export PASSWORD

bun -e "
import { Database } from 'bun:sqlite';
import { randomUUID } from 'crypto';

const username = process.env.USERNAME;
const password = process.env.PASSWORD;

try {
  const db = new Database('mal.db');
  
  const hash = await Bun.password.hash(password, { algorithm: 'bcrypt' });
  
  const id = randomUUID();
  
  const query = db.query('INSERT INTO user (id, username, password_hash) VALUES (\$id, \$username, \$hash)');
  
  query.run({
    \$id: id,
    \$username: username,
    \$hash: hash
  });
  
  console.log(\`✅ Brugeren '\${username}' blev oprettet med succes!\`);
} catch (error) {
  if (error.message.includes('UNIQUE constraint failed')) {
    console.error(\`❌ Fejl: Brugeren '\${username}' findes allerede.\`);
  } else {
    console.error('❌ Database fejl:', error.message);
  }
  process.exit(1);
}
"
