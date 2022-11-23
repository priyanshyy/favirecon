package favirecon

import (
	"bufio"
	"fmt"
	"os"
	"sync"

	"github.com/edoardottt/favirecon/pkg/input"
	"github.com/edoardottt/favirecon/pkg/output"
	"github.com/edoardottt/golazy"
	"github.com/projectdiscovery/gologger"
	fileutil "github.com/projectdiscovery/utils/file"
)

type Runner struct {
	Input     chan string
	Output    chan output.Found
	Result    output.Result
	UserAgent string
	InWg      *sync.WaitGroup
	OutWg     *sync.WaitGroup
	Options   input.Options
}

func New(options *input.Options) Runner {
	return Runner{
		Input:     make(chan string, options.Concurrency),
		Output:    make(chan output.Found, options.Concurrency),
		Result:    output.New(),
		UserAgent: golazy.GenerateRandomUserAgent(),
		InWg:      &sync.WaitGroup{},
		OutWg:     &sync.WaitGroup{},
		Options:   *options,
	}
}

func (r *Runner) Run() {
	r.InWg.Add(1)

	go pushInput(r)
	r.InWg.Add(1)

	go execute(r)
	r.OutWg.Add(1)

	go pullOutput(r)
	r.InWg.Wait()

	close(r.Output)
	r.OutWg.Wait()
}

func pushInput(r *Runner) {
	defer r.InWg.Done()

	if fileutil.HasStdin() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			r.Input <- scanner.Text()
		}
	}

	if r.Options.FileInput != "" {
		for _, line := range golazy.RemoveDuplicateValues(golazy.ReadFileLineByLine(r.Options.FileInput)) {
			r.Input <- line
		}
	}

	if r.Options.Input != "" {
		r.Input <- r.Options.Input
	}

	close(r.Input)
}

func execute(r *Runner) {
	defer r.InWg.Done()

	for i := 0; i < r.Options.Concurrency; i++ {
		r.InWg.Add(1)

		go func() {
			defer r.InWg.Done()

			for value := range r.Input {
				targetURL, err := prepareURL(value)
				if err != nil {
					if r.Options.Verbose {
						gologger.Error().Msgf("%s", err)
					}

					return
				}

				client := customClient(r.Options.Timeout)

				result, err := getFavicon(targetURL+"favicon.ico", r.UserAgent, client)
				if err != nil {
					if r.Options.Verbose {
						gologger.Error().Msgf("%s", err)
					}

					return
				}

				found, err := CheckFavicon(result, targetURL)
				if err != nil {
					if r.Options.Verbose {
						gologger.Error().Msgf("%s", err)
					}
				} else {
					r.Output <- output.Found{URL: targetURL, Name: found, Hash: result}
				}
			}
		}()
	}
}

func pullOutput(r *Runner) {
	defer r.OutWg.Done()

	for o := range r.Output {
		if !r.Result.Printed(o.URL) {
			r.OutWg.Add(1)

			go writeOutput(r.OutWg, &r.Options, o)
		}
	}
}

func writeOutput(wg *sync.WaitGroup, options *input.Options, o output.Found) {
	defer wg.Done()

	if options.FileOutput != "" && options.Output == nil {
		file, err := os.OpenFile(options.FileOutput, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0644)
		if err != nil {
			gologger.Fatal().Msg(err.Error())
		}

		options.Output = file
	}

	out := o.Format()

	if options.Output != nil {
		if _, err := options.Output.Write([]byte(out + "\n")); err != nil && options.Verbose {
			gologger.Fatal().Msg(err.Error())
		}
	}

	fmt.Println(out)
}
