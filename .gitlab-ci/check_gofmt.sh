#!/bin/sh

files="$(gofmt -l .)"

[ -z "$files" ] && exit 0

# run gofmt to print out the diff of what needs to be changed

gofmt -d -e .

exit 1
