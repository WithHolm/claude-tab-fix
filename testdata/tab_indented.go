package testdata

func example() {
	if true {
		x := 2
		_ = x
	}
}

func nested() {
	for i := 0; i < 10; i++ {
		if i%2 == 0 {
			println(i)
		}
	}
}
