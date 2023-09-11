PROFILES = profiles

all: echo.pb.go nopool pooled compare

nopool:
	$(MAKE) -B $(PROFILES)/nopool

pooled:
	$(MAKE) -B $(PROFILES)/pooled SEND_BUFFER_POOL=true

BENCHCOUNT = 10

$(PROFILES)/%:
	mkdir -p $(@D)
	go test \
		-o $@ \
		-count $(BENCHCOUNT) \
		-benchmem \
		-bench="BenchmarkServerThroughput" \
		-run NONE \
		-cpuprofile $(PROFILES)/$*.cpu \
		-memprofile $(PROFILES)/$*.mem \
		./ | tee $(PROFILES)/$*.txt
	go tool pprof $@ $(PROFILES)/$*.cpu <<< web
	go tool pprof -sample_index=alloc_space $(PROFILES)/$*.mem <<< web

benchstat:
	go install golang.org/x/perf/cmd/benchstat@latest

compare:
	benchstat $(PROFILES)/nopool.txt $(PROFILES)/pooled.txt

echo.pb.go: echo.proto
	protoc --proto_path=. \
		--go_opt=paths=source_relative \
		--go_out=. \
		--go-grpc_opt=paths=source_relative \
		--go-grpc_out=. \
		echo.proto
