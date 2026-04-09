package orphan

// This file has no go.mod/package.json/etc. anywhere up the tree.
// The parser falls back to repo:<account>/<repo> identity.

func Lonely() int { return 1 }

func Also() string { return "no manifest" }
