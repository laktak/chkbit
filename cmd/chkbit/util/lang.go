package util

import "fmt"

func LangNum1MutateSuffix(num int, u string) string {
	s := ""
	if num != 1 {
		s = "s"
	}
	return fmt.Sprintf("%d %s%s", num, u, s)
}

func LangNum1Choice(num int, u1, u2 string) string {
	u := u1
	if num != 1 {
		u = u2
	}
	return fmt.Sprintf("%d %s", num, u)
}
