package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"text/template"
)

const TEMPLATE_FILE = "main.py.tmpl"

func main() {
	http.HandleFunc("/create_model", handleCreateModel)

	port := os.Getenv("PORT")
	if port == "" {
		port = "1212"
	}
	log.Printf("Server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

type Request struct {
	ModelCode string `json:"model_code"`
}

func handleCreateModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading request body", http.StatusBadRequest)
		return
	}

	log.Printf("Received request body: %s", string(bodyBytes))

	// Close the original body and create a new one with the same data
	r.Body.Close()
	r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	var req Request
	err = json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		log.Printf("JSON decode error: %v", err)
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	log.Printf("Successfully parsed request: ModelCode=\n```\n%s\n```\n", req.ModelCode)

	tempFile, err := os.CreateTemp("", "model*.glb")
	if err != nil {
		log.Printf("Error creating temp file: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	tempFilePath := tempFile.Name()
	tempFile.Close()
	log.Printf("Using temporary file path: %s", tempFilePath)
	defer os.Remove(tempFilePath)

	model_expression, err := create_glb(req.ModelCode, tempFilePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing templates: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Python expression generated.")

	tmpFile, err := os.CreateTemp("", "blender_*.py")
	if err != nil {
		log.Printf("Error creating temp file: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.WriteString(model_expression)
	if err != nil {
		log.Printf("Error writing to temp file: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	tmpFile.Close()

	cmd := exec.Command("blender", "-b", "--python", tmpFile.Name())
	output, err := cmd.CombinedOutput()
	blenderOutput := string(output)
	if err != nil || strings.Contains(blenderOutput, "Traceback") || strings.Contains(blenderOutput, "Error:") {
		log.Printf("Command execution failed: %v, Output: %s", err, blenderOutput)
		if err == nil {
			err = errors.New("Python error")
		}
		errorResponse := struct {
			Error      string `json:"error"`
			BlenderLog string `json:"blender_log"`
			ExitCode   string `json:"exit_code"`
		}{
			Error:      "Blender render failed",
			BlenderLog: blenderOutput,
			ExitCode:   err.Error(),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(errorResponse)
		return
	}

	fileInfo, err := os.Stat(tempFilePath)
	if os.IsNotExist(err) || fileInfo.Size() == 0 {
		log.Printf("Error: GLB file was not created or is empty at %s", tempFilePath)

		errorResponse := struct {
			Error      string `json:"error"`
			Details    string `json:"details"`
			BlenderLog string `json:"blender_log"`
		}{
			Error:      "Failed to generate GLB file",
			Details:    "The Blender process completed but did not generate a valid model file. This typically indicates an error in the model code.",
			BlenderLog: string(output),
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(errorResponse)
		return
	}

	log.Printf("Blender command executed; output: %s", string(output))

	glbData, err := os.ReadFile(tempFilePath)
	if err != nil {
		log.Printf("Error reading GLB file: %v", err)
		http.Error(w, "Error reading generated GLB file", http.StatusInternalServerError)
		return
	}

	log.Printf("GLB file read successfully, size: %d bytes", len(glbData))

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=model.glb"))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(glbData)))

	if _, err := w.Write(glbData); err != nil {
		log.Printf("Failed to write response: %v", err)
	} else {
		log.Printf("GLB file sent successfully")
	}
}

type ModelTemplateVars struct {
	ModelCode string
	Filename  string
}

func create_glb(model_code string, filename string) (string, error) {
	t := template.New(TEMPLATE_FILE)

	t, err := t.ParseFiles(TEMPLATE_FILE)
	if err != nil {
		fmt.Println("Error parsing templates:", err)
		return "", err
	}

	var buf bytes.Buffer
	data := ModelTemplateVars{
		ModelCode: model_code,
		Filename:  filename,
	}

	err = t.ExecuteTemplate(&buf, TEMPLATE_FILE, data)
	if err != nil {
		fmt.Println("Error executing template:", err)
		return "", err
	}

	result := buf.String()
	return result, nil
}
