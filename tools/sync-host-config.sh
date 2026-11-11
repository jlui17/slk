#!/bin/sh
# Makes the state volume's config.toml carry the host config's values for the
# keys named in SLK_HOST_CONFIG_KEYS (run-docker.sh sets the list), each value
# passed in by name as SLK_HOST_CONFIG_<key>: a key rotated or changed on the
# host reaches the docker sessions on their next launch, and a key deleted
# from the host config is deleted from the volume's copy, so the sessions
# switch to the ant login (anthropic_api_key) or to the default (the judge
# keys) instead of keeping the old value. A key the volume lacks is added
# under [herdr]. run-docker.sh runs it inside the slk container just before
# slk starts.
#
# Only these key lines move: the volume's config was seeded once from the host
# and slk has saved its own settings into it since (themes, sidebar widths,
# version_ts), which a copy of the whole host file would throw away.
# The sed below assumes no value has '|', '&' or '\', which Anthropic keys
# and model names never carry.
set -eu

config=$XDG_CONFIG_HOME/slk/config.toml
[ -f "$config" ] || exit 0

edits=
changed=
for name in $SLK_HOST_CONFIG_KEYS; do
  value=$(eval "printf %s \"\${SLK_HOST_CONFIG_$name:-}\"")
  line="$name = \"$value\""
  if [ -z "$value" ]; then
    grep -q "^$name *=" "$config" || continue
    edits="$edits
/^$name *=/d"
    changed="$changed $name deleted: the host config has none;"
  elif grep -q "^$name *=" "$config"; then
    grep -qxF "$line" "$config" && continue
    edits="$edits
s|^$name *= *\".*\"\$|$line|"
    changed="$changed $name copied from the host config;"
  else
    if ! grep -q '^\[herdr\]' "$config"; then
      echo "slk: $config has no [herdr] section to add $name to" >&2
      continue
    fi
    edits="$edits
/^\[herdr\]/a $line"
    changed="$changed $name added from the host config;"
  fi
done
[ -n "$edits" ] || exit 0

# The lock every slk saver holds for its read-modify-write of the config.
exec 9>"$config.lock"
flock -w 2 9
# Same directory as the config, so the rename below is atomic.
tmp=$(mktemp "$config.XXXXXX")
trap 'rm -f "$tmp"' EXIT
chmod --reference="$config" "$tmp"
sed "$edits" "$config" >"$tmp"
mv "$tmp" "$config"
echo "slk: state volume's config:$changed" >&2
