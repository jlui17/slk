# README hero: navigation, fuzzy finder, live activity, a split, workspaces.
Output docs/assets/demo/hero.gif
Source docs/assets/demo/settings.tape

Hide
Type "./bin/slk --demo hero"
Enter
Sleep 1.5s
Show

# Move through #general with vim keys
Sleep 1s
Type@350ms "kkkjj"
Sleep 700ms

# Jump to #engineering: Priya types, posts "This is fine", and Sam reacts
Ctrl+T
Sleep 500ms
Type@120ms "eng"
Sleep 500ms
Enter
Sleep 6s

# Split vertically and put #deploys alongside
Ctrl+W
Type "v"
Sleep 700ms
Ctrl+T
Sleep 400ms
Type@120ms "depl"
Sleep 400ms
Enter
Sleep 2.5s

# Driftwood OSS lit up in the rail meanwhile: hop over and back
Type "2"
Sleep 2.5s
Type "1"
Sleep 1.5s
