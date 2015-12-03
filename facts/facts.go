/*
   Fact-Gathering functions

   Copyright 2015 - Matt Oswalt
*/

package facts

import (
    "bytes"
    "encoding/json"
    "fmt"
    "io/ioutil"
    "net/http"
    "os"
    "os/exec"
    "path/filepath"

    "github.com/mierdin/todd/assets"
    "github.com/mierdin/todd/common"
    "github.com/mierdin/todd/config"
)

// GetFacts is responsible for gathering facts on a system (runs on the agent).
// It does so by iterating over the collectors installed on this agent's system,
// executing them, and capturing their output. It will aggregate this output and
// return it all as a single map (keys are fact names)
func GetFacts(cfg config.Config) map[string]interface{} {

    retFactSet := make(map[string]interface{})

    // this is the function that will do work on a single file during a walk
    execute_collector := func(path string, f os.FileInfo, err error) error {
        if f.IsDir() != true {
            cmd := exec.Command(path)

            // Stdout buffer
            cmdOutput := &bytes.Buffer{}
            // Attach buffer to command
            cmd.Stdout = cmdOutput
            // Execute collector
            cmd.Run()

            // We only care that the key is a string (this is the fact name)
            // The value for this key can be whatever
            fact := make(map[string]interface{})

            // Unmarshal JSON into our fact map
            err = json.Unmarshal(cmdOutput.Bytes(), &fact)

            // We only expect a single key in the returned fact map. Only add to fact map if this is true.
            if len(fact) == 1 {
                for factName, factValue := range fact {
                    retFactSet[factName] = factValue
                }
            }
        }
        return nil
    }

    // Perform above Walk function (execute_collector) on the collector directory
    err := filepath.Walk(cfg.Facts.CollectorDir, execute_collector)
    common.FailOnError(err, "Problem running fact-gathering collector")

    return retFactSet
}

// GetFactCollectors gathers all currently installed collectors, and generates a map of their names and hashes
func GetFactCollectors(cfg config.Config) map[string]string {

    // Initialize collector map
    collectors := make(map[string]string)

    // this is the function that will generate a hash for a file and add it to our collector map
    discover_collector := func(path string, f os.FileInfo, err error) error {

        if f.IsDir() != true {
            // Generate hash
            collectors[f.Name()] = common.GetFileSHA256(path)
        }
        return nil
    }

    // create fact collector directory if needed
    err := os.MkdirAll(cfg.Facts.CollectorDir, 0777)
    common.FailOnError(err, "Problem creating fact collector directory")

    // Perform above Walk function (execute_collector) on the collector directory
    err = filepath.Walk(cfg.Facts.CollectorDir, discover_collector)
    common.FailOnError(err, "error")

    // TODO(moswalt): Should we include a mechanism to delete unused collectors? The current design only fixes
    // missing or incorrect collectors, not extra ones.

    // Return collectors so that the calling function can pass this to the registry for enforcement
    return collectors

}

// ServeFactCollectors is responsible for deriving collector files from the embedded golang source generated by go-bindata
// These will be written to the collector directory, a hash (SHA256) will be generated, and these files will be served via HTTP
// This function is typically run on the ToDD server.
func ServeFactCollectors(cfg config.Config) map[string]string {

    // Initialize collector map
    collector_map := make(map[string]string)

    // TODO(moswalt): Temporary measure - should figure out a way to iterate over these in the Asset
    collector_map["get_interfaces"] = ""
    collector_map["get_hostname"] = ""

    // create fact collector directory if needed
    err := os.MkdirAll(cfg.Facts.CollectorDir, 0777)
    common.FailOnError(err, "Problem creating fact collector directory")

    for name, _ := range collector_map {

        // Retrieve collector Asset from embedded Go source
        data, err := assets.Asset(fmt.Sprintf("facts/collectors/%s", name))
        common.FailOnError(err, "Error retrieving collector from embedded source")

        // Derive full path to collector file, and write it
        file := fmt.Sprintf("%s/%s", cfg.Facts.CollectorDir, name)
        err = ioutil.WriteFile(file, data, 0744)
        common.FailOnError(err, "Error writing collector file")

        // Generate SHA256 for this collector file, and append to collector map
        collector_map[name] = common.GetFileSHA256(file)
    }

    // Begin serving files to agents
    // TODO(moswalt): Handle error
    go http.ListenAndServe(fmt.Sprintf(":%s", cfg.Facts.Port), http.FileServer(http.Dir(cfg.Facts.CollectorDir)))

    // Return collector_map so that the calling function can pass this to the registry for enforcement
    return collector_map

}