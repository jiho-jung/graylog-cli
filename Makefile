
# config
-include makefile.cfg

build:
	go build

args =

ifneq ($(endpoint),)
	args += --server $(endpoint)
endif

ifneq ($(username),)
	args += --username $(username)
endif

ifneq ($(password),)
	args += --password $(password)
endif

run:
	./graylog-cli search ${args} -p
#	./graylog-cli search ${args}
