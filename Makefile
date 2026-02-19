# Makefile
.PHONY: run analyze-last-year analyze-this-year

run:
	go run main.go

# 昨年Q4のデータ
analyze-2025-q4:
	go run main.go -start 2025-10-01 -end 2025-12-31

# 今年のデータ
analyze-2026-all:
	go run main.go -start 2026-01-01 -end 2026-02-19
