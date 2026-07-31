// Temporary timing probe: how long does the hub SHACL pass take with and
// without the extra active shape libraries the dev hub carries.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/tggo/goRDFlib/shacl"
)

func read(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func timeValidate(label, shapesTTL, contractJSON string) {
	start := time.Now()
	shapesGraph, err := shacl.LoadTurtleString(shapesTTL, "urn:dcs:hub:shapes")
	if err != nil {
		fmt.Printf("%-24s LOAD ERROR %v\n", label, err)
		return
	}
	shapesGraph.Triples()
	loaded := time.Since(start)

	dataGraph, err := shacl.LoadJsonLDString(contractJSON, "urn:dcs:contract")
	if err != nil {
		fmt.Printf("%-24s DATA ERROR %v\n", label, err)
		return
	}

	vstart := time.Now()
	shacl.Validate(dataGraph, shapesGraph)
	fmt.Printf("%-24s shapes=%7d bytes  parse=%8s  validate=%8s\n",
		label, len(shapesTTL), loaded.Round(time.Millisecond), time.Since(vstart).Round(time.Millisecond))
}

func main() {
	dir := os.Args[1]
	core := read(dir + "/shapes_core.ttl")
	catalog := read(dir + "/shapes_catalog.ttl")
	e2e := read(dir + "/shapes_e2e.ttl")
	gaiax := read(dir + "/shapes_gaiax.ttl")
	contract := read(dir + "/contract.json")

	timeValidate("core+catalog", core+"\n\n"+catalog, contract)
	timeValidate("core+catalog+e2e", core+"\n\n"+catalog+"\n\n"+e2e, contract)
	timeValidate("core+catalog+e2e+gaiax", core+"\n\n"+catalog+"\n\n"+e2e+"\n\n"+gaiax, contract)
}
