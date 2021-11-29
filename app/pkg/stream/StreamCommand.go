package stream

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"strings"
	"time"

	"pluralith/pkg/comdb"
	"pluralith/pkg/ux"
)

func StreamCommand(command string, args []string) error {
	// Instantiate spinners
	streamSpinner := ux.NewSpinner("Apply Running", "Apply Completed", "Apply Failed")
	// Adapting spinner to destroy command
	if command == "destroy" {
		streamSpinner = ux.NewSpinner("Destroy Running", "Destroy Completed", "Destroy Failed")
	}

	// Get working directory for update emission
	workingDir, workingErr := os.Getwd()
	if workingErr != nil {
		return workingErr
	}

	// Emit apply begin update to UI
	comdb.PushComDBEvent(comdb.Event{
		Receiver:   "UI",
		Timestamp:  time.Now().Unix(),
		Command:    "apply",
		Type:       "begin",
		Address:    "",
		Attributes: make(map[string]interface{}),
		Path:       workingDir,
		Received:   false,
	})

	streamSpinner.Start()
	// Constructing command to execute
	cmd := exec.Command("terraform", append([]string{"apply"}, args...)...)

	// Define sinks for std data
	var errorSink bytes.Buffer

	// Redirect command std data
	cmd.Stderr = &errorSink

	// Initiate standard output pipe
	outStream, outErr := cmd.StdoutPipe()
	if outErr != nil {
		streamSpinner.Fail()
		return outErr
	}

	// Run terraform command
	cmdErr := cmd.Start()
	if cmdErr != nil {
		streamSpinner.Fail()
		return cmdErr
	}

	// Scan for command line updates
	applyScanner := bufio.NewScanner(outStream)
	applyScanner.Split(bufio.ScanLines)

	// While command line scan is running
	for applyScanner.Scan() {
		// Get current line json string
		jsonString := applyScanner.Text()
		// Decode json string to get event type and resource address
		event, address, decodeErr := DecodeStateStream(jsonString)
		if decodeErr != nil {
			streamSpinner.Fail()
			return decodeErr
		}

		// If address is given
		if address != "" {
			// Fetch current tfstate from state file and strip secrets
			fetchedState, resourceFound, fetchErr := FetchState(address, false)
			if fetchErr != nil {
				return fetchErr
			}

			if resourceFound {
				FetchResourceAttributes(address, fetchedState)
			}

			// NOT NECESSARY -> Update plan json and UI will watch those file changes
			// // Emit current event update to UI
			comdb.PushComDBEvent(comdb.Event{
				Receiver:   "UI",
				Timestamp:  time.Now().Unix(),
				Command:    "apply",
				Type:       strings.Split(event, "_")[1],
				Address:    address,
				Attributes: make(map[string]interface{}),
				Path:       workingDir,
				Received:   false,
			})
		}
	}

	// Emit apply start update to UI
	comdb.PushComDBEvent(comdb.Event{
		Receiver:   "UI",
		Timestamp:  time.Now().Unix(),
		Command:    "apply",
		Type:       "end",
		Address:    "",
		Attributes: make(map[string]interface{}),
		Path:       workingDir,
		Received:   false,
	})

	streamSpinner.Success()

	return nil
}