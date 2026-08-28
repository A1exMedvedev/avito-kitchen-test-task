package domain

const Currency = "RUB"

type Money int64

func (m Money) Add(other Money) Money { return m + other }

func (m Money) Times(quantity int) Money { return m * Money(quantity) }

func (m Money) IsNegative() bool { return m < 0 }
