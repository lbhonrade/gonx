package gonx

import (
	"bufio"
	"io"
	"sync"
)

type MapReduceEntry struct {
	entryString string
	lineIndex uint64
}

func handleError(err error) {
	//fmt.Fprintln(os.Stderr, err)
}

// Iterate over given file and map each it's line into Entry record using
// parser and apply reducer to the Entries channel. Execution terminates
// when result will be readed from reducer's output channel, but the mapper
// works and fills input Entries channel until all lines will be read from
// the fiven file.
func MapReduce(file io.Reader, parser *Parser, reducer Reducer) chan *Entry {
	// Input file lines. This channel is unbuffered to publish
	// next line to handle only when previous is taken by mapper.
	var lines = make(chan *MapReduceEntry)

	// Host thread to spawn new mappers
	var entries = make(chan *Entry, 10)
	go func(topLoad int) {
		// Create semafore channel with capacity equal to the output channel
		// capacity. Use it to control mapper goroutines spawn.
		var sem = make(chan bool, topLoad)
		for i := 0; i < topLoad; i++ {
			// Ready to go!
			sem <- true
		}

		var wg sync.WaitGroup
		for {
			// Wait until semaphore becomes available and run a mapper
			if !<-sem {
				// Stop the host loop if false received from semaphore
				break
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				// Take next file line to map. Check is channel closed.
				line, ok := <-lines
				// Return immediately if lines channel is closed
				if !ok {
					// Send false to semaphore channel to indicate that job's done
					sem <- false
					return
				}
				entry, err := parser.ParseString(line.entryString)
				entry.entryIndex = line.lineIndex
				if err == nil {
					// Write result Entry to the output channel. This will
					// block goroutine runtime until channel is free to
					// accept new item.
					entries <- entry
				} else {
					handleError(err)
				}
				// Increment semaphore to allow new mapper workers to spawn
				sem <- true
			}()
		}
		// Wait for all mappers to complete, then send a quit signal
		wg.Wait()
		close(entries)
	}(cap(entries))

	// Run reducer routine.
	var output = make(chan *Entry)
	go reducer.Reduce(entries, output)

	go func() {
		scanner := bufio.NewScanner(file)
		lineIndexCtr := uint64(1)
		for scanner.Scan() {
			// Read next line from the file and feed mapper routines.
			lines <- &MapReduceEntry{
				entryString: scanner.Text(),
				lineIndex: lineIndexCtr,
			}
			lineIndexCtr += uint64(1)
		}
		close(lines)

		if err := scanner.Err(); err != nil {
			handleError(err)
		}
	}()

	return output
}
