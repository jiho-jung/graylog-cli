
# config
#-include makefile.cfg

build:
	go build

#args =
#ifneq ($(endpoint),)
#	args += --server $(endpoint)
#endif
#
#ifneq ($(username),)
#	args += --username $(username)
#endif
#
#ifneq ($(password),)
#	args += --password $(password)
#endif

run:
#	./graylog-cli search ${args} --since 8h --page
#	./graylog-cli search ${args}
	./graylog-cli search --since 1m  --tui
