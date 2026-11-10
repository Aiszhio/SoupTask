package main

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	jobsv1 "SoupWebApplication/pkg/api/jobs/v1"
)

var wordBoundary = regexp.MustCompile("([a-z0-9])([A-Z])")

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	service := jobsv1.File_jobs_v1_jobs_proto.Services().ByName("JobService")
	if service == nil {
		return fmt.Errorf("JobService is missing from generated proto")
	}

	if err := os.MkdirAll(filepath.Join("internal", "app"), 0755); err != nil {
		return err
	}

	methods := service.Methods()
	for i := 0; i < methods.Len(); i++ {
		method := methods.Get(i)
		name := string(method.Name())
		fileName := "v1_" + strings.ToLower(wordBoundary.ReplaceAllString(name, "${1}_${2}")) + ".go"
		path := filepath.Join("internal", "app", fileName)

		source := fmt.Sprintf(handlerTemplate, name, name, name, method.Input().Name(), method.Output().Name(), name)
		formatted, err := format.Source([]byte(source))
		if err != nil {
			return fmt.Errorf("format %s: %w", path, err)
		}

		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if os.IsExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}

		_, writeErr := file.Write(formatted)
		closeErr := file.Close()
		if writeErr != nil {
			return fmt.Errorf("write %s: %w", path, writeErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close %s: %w", path, closeErr)
		}
		fmt.Println("created", path)
	}

	return nil
}

const handlerTemplate = "package app\n\n" +
	"import (\n" +
	"\t\"context\"\n\n" +
	"\tjobsv1 \"SoupWebApplication/pkg/api/jobs/v1\"\n\n" +
	"\t\"google.golang.org/grpc/codes\"\n" +
	"\t\"google.golang.org/grpc/status\"\n" +
	")\n\n" +
	"// %s - handles %s requests\n" +
	"func (service *Service) %s(_ context.Context, _ *jobsv1.%s) (*jobsv1.%s, error) {\n" +
	"\treturn nil, status.Error(codes.Unimplemented, \"method %s is not implemented\")\n" +
	"}\n"
