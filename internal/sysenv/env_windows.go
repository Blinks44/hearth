package sysenv

import "strings"

func sameKey(a, b string) bool { return strings.EqualFold(a, b) }
