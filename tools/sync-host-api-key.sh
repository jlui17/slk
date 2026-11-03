#!/bin/sh
# Makes the state volume's config.toml carry the host config's
# anthropic_api_key, passed in by name as SLK_HOST_ANTHROPIC_API_KEY: a key
# rotated on the host reaches the docker sessions on their next launch, and
# a key deleted from the host config is deleted from the volume's copy, so
# the sessions switch to the ant login instead of keeping the old key.
# run-docker.sh runs it inside the slk container just before slk starts.
#
# Only the key line moves: the volume's config was seeded once from the host
# and slk has saved its own settings into it since (themes, sidebar widths,
# version_ts), which a copy of the whole host file would throw away.
# The sed below assumes the key has no '|', '&' or '\', which Anthropic keys
# never carry.
set -eu

key=${SLK_HOST_ANTHROPIC_API_KEY:-}
config=$XDG_CONFIG_HOME/slk/config.toml
line="anthropic_api_key = \"$key\""

[ -f "$config" ] || exit 0
if [ -z "$key" ]; then
  grep -q '^anthropic_api_key *=' "$config" || exit 0
  edit='/^anthropic_api_key *=/d'
  done="deleted anthropic_api_key from the state volume's config: the host config has none"
else
  if ! grep -q '^anthropic_api_key *=' "$config"; then
    echo "slk: $config has no anthropic_api_key line to update; add it under [herdr]" >&2
    exit 0
  fi
  grep -qxF "$line" "$config" && exit 0
  edit="s|^anthropic_api_key *= *\".*\"\$|$line|"
  done="copied anthropic_api_key from the host config into the state volume"
fi

# The lock every slk saver holds for its read-modify-write of the config.
exec 9>"$config.lock"
flock -w 2 9
# Same directory as the config, so the rename below is atomic.
tmp=$(mktemp "$config.XXXXXX")
trap 'rm -f "$tmp"' EXIT
chmod --reference="$config" "$tmp"
sed "$edit" "$config" >"$tmp"
mv "$tmp" "$config"
echo "slk: $done" >&2
