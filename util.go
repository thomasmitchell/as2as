package main

type StringList []string

func (l StringList) Contains(s string) bool {
	for i := range l {
		if l[i] == s {
			return true
		}
	}

	return false
}
