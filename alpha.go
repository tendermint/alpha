package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	crypto "github.com/tendermint/go-crypto"
	"github.com/tendermint/tendermint/types"
)

const tpl = `
<!doctype html>

<html lang="en">
<head>
	<meta charset="utf-8">

	<title>Tendermint Alpha</title>
	<meta name="description" content="Tiny web app to help you form a genesis file">
	<meta name="author" content="Anton Kaliaev">

	<link href="https://fonts.googleapis.com/css?family=Slabo+27px" rel="stylesheet">
	<link rel="stylesheet" href="https://unpkg.com/blaze/scss/dist/components.buttons.min.css">
	<link rel="stylesheet" href="https://unpkg.com/blaze/scss/dist/components.inputs.min.css">

	<style>
		body {
			font-family: 'Slabo 27px', serif;
			font-size: 21px;
		}
	</style>

	<!--[if lt IE 9]>
		<script src="https://cdnjs.cloudflare.com/ajax/libs/html5shiv/3.7.3/html5shiv.js"></script>
	<![endif]-->
</head>

<body>
	<div style="width:40%; margin:0 auto;">
		{{.}}
	</div>
</body>
</html>`

var (
	genesisDocs              = make(map[string]types.GenesisDoc)
	errorEmptyValidatorField = errors.New("Please fill in all fields")
)

var pageTemplate = template.Must(template.New("page").Parse(tpl))

type errorGenesisNotFound struct {
	chainID string
}

func (e errorGenesisNotFound) Error() string {
	return fmt.Sprintf("genesis with such chain ID %s not found", e.chainID)
}

func listHandler(w http.ResponseWriter, r *http.Request) {
	var b strings.Builder
	fmt.Fprintf(&b, "<h1>Genesis files</h1>")
	fmt.Fprintf(&b, "<ul>")
	for chainID := range genesisDocs {
		fmt.Fprintf(&b, "<li><h3><a href=\"/view/%s\">%s</a></h3></li>", chainID, chainID)
	}
	if len(genesisDocs) == 0 {
		fmt.Fprintf(&b, "<li><h3>No genesis files :(<h3></li>")
	}
	fmt.Fprintf(&b, "</ul>")
	fmt.Fprintf(&b, "<a href=\"/new\" class=\"c-button c-button--info\">New</a>")

	err := pageTemplate.Execute(w, template.HTML(b.String()))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func newHandler(w http.ResponseWriter, r *http.Request) {
	data := "<h1>New genesis</h1>" +
		"<form action=\"/create\" method=\"POST\">" +
		"ChainID (*) <input type=\"text\" name=\"chainID\" class=\"c-field\" required><br>" +
		// "ConsensusParams (not working now) <textarea name=\"consensus_params\"></textarea><br>"+
		"Your Validator PubKey (raw json; output of `tendermint show_validator`) (optional) <textarea name=\"validator_pubkey\" rows=\"6\" class=\"c-field\"></textarea><br>" +
		"Your Validator Power (optional) <input type=\"number\" name=\"validator_power\" class=\"c-field\"><br>" +
		"Your Validator Name (optional) <input type=\"text\" name=\"validator_name\" class=\"c-field\"><br>" +
		"App Hash (optional) <input type=\"text\" name=\"app_hash\" class=\"c-field\"><br>" +
		"App State (raw json) (optional) <textarea name=\"app_state\" rows=\"6\" class=\"c-field\"></textarea><br>" +
		"<input type=\"submit\" value=\"Create\" class=\"c-button c-button--info\">" +
		"</form>"

	err := pageTemplate.Execute(w, template.HTML(data))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func createHandler(w http.ResponseWriter, r *http.Request) {
	var chainIDRegexp = regexp.MustCompile("^[a-zA-Z0-9-_]+$")
	chainID := r.FormValue("chainID")
	if chainID == "" {
		http.Error(w, "chainID is required", http.StatusNotAcceptable)
		return
	} else if !chainIDRegexp.MatchString(chainID) {
		http.Error(w, "please keep it simple and stick with [a-zA-Z0-9-_] for chainID", http.StatusNotAcceptable)
		return
	} else if _, ok := genesisDocs[chainID]; ok {
		http.Error(w, fmt.Sprintf("genesis file for such chainID (%q) already exist", chainID), http.StatusNotAcceptable)
		return
	}

	// TODO allow changing
	consensusParams := types.DefaultConsensusParams()

	validators := []types.GenesisValidator{}

	validator, err := buildValidator(r)
	if err != nil && err != errorEmptyValidatorField {
		http.Error(w, err.Error(), http.StatusNotAcceptable)
		return
	} else if err == nil {
		validators = append(validators, validator)
	}

	appHash := r.FormValue("app_hash")
	appState := json.RawMessage([]byte(r.FormValue("app_state")))

	genesisDocs[chainID] = types.GenesisDoc{
		GenesisTime:     time.Now(),
		ChainID:         chainID,
		ConsensusParams: consensusParams,
		Validators:      validators,
		AppHash:         []byte(appHash),
		AppStateJSON:    appState,
	}

	data := fmt.Sprintf("<h1>Give <a href=\"/new_validator/%s\">this link</a> to other validators && <a href=\"/view/%s\">view genesis JSON</a></h1>", chainID, chainID)

	err = pageTemplate.Execute(w, template.HTML(data))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

var validPath = regexp.MustCompile("^/(new_validator|add_validator|view|download)/([a-zA-Z0-9_-]+)$")

func makeHandler(fn func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m := validPath.FindStringSubmatch(r.URL.Path)
		if m == nil {
			http.NotFound(w, r)
			return
		}
		fn(w, r, m[2])
	}
}

func newValidatorHandler(w http.ResponseWriter, r *http.Request, chainID string) {
	_, ok := genesisDocs[chainID]
	if !ok {
		http.Error(w, errorGenesisNotFound{chainID}.Error(), http.StatusNotFound)
		return
	}

	data := fmt.Sprintf("<h1>Add validator</h1>"+
		"<form action=\"/add_validator/%s\" method=\"POST\">"+
		"Your Validator PubKey (raw json; output of `tendermint show_validator`) <textarea name=\"validator_pubkey\" rows=\"6\" class=\"c-field\" required></textarea><br>"+
		"Your Validator Power <input type=\"number\" name=\"validator_power\" class=\"c-field\" required><br>"+
		"Your Validator Name <input type=\"text\" name=\"validator_name\" class=\"c-field\" required><br>"+
		"<input type=\"submit\" value=\"Add\" class=\"c-button c-button--info\">"+
		"</form>", chainID)

	err := pageTemplate.Execute(w, template.HTML(data))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func addValidatorHandler(w http.ResponseWriter, r *http.Request, chainID string) {
	genDoc, ok := genesisDocs[chainID]
	if !ok {
		http.Error(w, errorGenesisNotFound{chainID}.Error(), http.StatusNotFound)
		return
	}

	validator, err := buildValidator(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotAcceptable)
		return
	}

	genDoc.Validators = append(genDoc.Validators, validator)
	sort.Slice(genDoc.Validators, func(i, j int) bool {
		return genDoc.Validators[i].Power > genDoc.Validators[j].Power
	})
	genesisDocs[chainID] = genDoc
	http.Redirect(w, r, "/view/"+chainID, http.StatusFound)
}

func viewHandler(w http.ResponseWriter, r *http.Request, chainID string) {
	genDoc, ok := genesisDocs[chainID]
	if !ok {
		http.Error(w, errorGenesisNotFound{chainID}.Error(), http.StatusNotFound)
		return
	}

	json, err := json.MarshalIndent(genDoc, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var b strings.Builder
	fmt.Fprintf(&b, "<h1>%d validators have checked in so far</h1>", len(genDoc.Validators))
	fmt.Fprintf(&b, "<a href=\"/download/%s\" class=\"c-button c-button--info\">Download genesis file</a>", chainID)
	fmt.Fprintf(&b, "<pre style=\"font-size:16px;\"><code>%s</code></pre>", string(json))
	err = pageTemplate.Execute(w, template.HTML(b.String()))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func downloadHandler(w http.ResponseWriter, r *http.Request, chainID string) {
	genDoc, ok := genesisDocs[chainID]
	if !ok {
		http.Error(w, errorGenesisNotFound{chainID}.Error(), http.StatusNotFound)
		return
	}

	json, err := json.MarshalIndent(genDoc, "", "  ")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Disposition", "attachment;")
	w.Header().Set("Content-Type", "application/json")

	fmt.Fprintf(w, "%s", string(json))
}

func buildValidator(r *http.Request) (types.GenesisValidator, error) {
	if r.FormValue("validator_pubkey") == "" || r.FormValue("validator_power") == "" || r.FormValue("validator_name") == "" {
		return types.GenesisValidator{}, errorEmptyValidatorField
	}

	power, err := strconv.ParseInt(r.FormValue("validator_power"), 10, 64)
	if err != nil {
		return types.GenesisValidator{}, err
	} else if power < 0 {
		return types.GenesisValidator{}, errors.New("Power can't be negative")
	}

	var pubKey crypto.PubKey
	err = json.Unmarshal([]byte(r.FormValue("validator_pubkey")), &pubKey)
	if err != nil {
		return types.GenesisValidator{}, fmt.Errorf("Failed to parse PubKey: %v", err)
	}

	return types.GenesisValidator{
		PubKey: pubKey,
		Power:  power,
		Name:   r.FormValue("validator_name"),
	}, nil
}

func main() {
	http.HandleFunc("/", listHandler)

	http.HandleFunc("/new", newHandler)
	http.HandleFunc("/create", createHandler)

	http.HandleFunc("/new_validator/", makeHandler(newValidatorHandler))
	http.HandleFunc("/add_validator/", makeHandler(addValidatorHandler))
	http.HandleFunc("/view/", makeHandler(viewHandler))
	http.HandleFunc("/download/", makeHandler(downloadHandler))

	log.Fatal(http.ListenAndServe(":8080", nil))
}
