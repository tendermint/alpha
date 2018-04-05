package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"regexp"
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

	<style>
	</style>

	<!--[if lt IE 9]>
		<script src="https://cdnjs.cloudflare.com/ajax/libs/html5shiv/3.7.3/html5shiv.js"></script>
	<![endif]-->
</head>

<body>
	{{.}}
</body>
</html>`

var (
	genesisDocs              = make(map[string]types.GenesisDoc)
	errorEmptyValidatorField = errors.New("incorrect validator fields")
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
	for chainID := range genesisDocs {
		fmt.Fprintf(&b, "<h1><a href=\"/view/%s\">%s</a></h1>", chainID, chainID)
	}
	fmt.Fprintf(&b, "<a href=\"/new\">New</a>")

	err := pageTemplate.Execute(w, template.HTML(b.String()))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func newHandler(w http.ResponseWriter, r *http.Request) {
	data := "<h1>New genesis</h1>" +
		"<form action=\"/create\" method=\"POST\">" +
		"ChainID <input type=\"text\" name=\"chainID\" required><br>" +
		// "ConsensusParams (not working now) <textarea name=\"consensus_params\"></textarea><br>"+
		"Your Validator PubKey (raw json; output of `tendermint show_validator`) (optional) <textarea name=\"validator_pubkey\"></textarea><br>" +
		"Your Validator Power (optional) <input type=\"number\" name=\"validator_power\"><br>" +
		"Your Validator Name (optional) <input type=\"text\" name=\"validator_name\"><br>" +
		"App Hash (optional) <input type=\"text\" name=\"app_hash\"><br>" +
		"App State (raw json) (optional) <textarea name=\"app_state\"></textarea><br>" +
		"<input type=\"submit\" value=\"Create\">" +
		"</form>"

	err := pageTemplate.Execute(w, template.HTML(data))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func createHandler(w http.ResponseWriter, r *http.Request) {
	chainID := r.FormValue("chainID")

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

var validPath = regexp.MustCompile("^/(new_validator|add_validator|view)/([a-zA-Z0-9]+)$")

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
		"Your Validator PubKey (raw json; output of `tendermint show_validator`) <textarea name=\"validator_pubkey\"></textarea><br>"+
		"Your Validator Power <input type=\"number\" name=\"validator_power\"><br>"+
		"Your Validator Name <input type=\"text\" name=\"validator_name\"><br>"+
		"<input type=\"submit\" value=\"Add\">"+
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
	}

	var pubKey crypto.PubKey
	err = json.Unmarshal([]byte(r.FormValue("validator_pubkey")), &pubKey)
	if err != nil {
		return types.GenesisValidator{}, err
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

	log.Fatal(http.ListenAndServe(":8080", nil))
}
