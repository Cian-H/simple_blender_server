package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

func main() {
	http.HandleFunc("/create_model", handleCreateModel)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("Server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

const ModelPath = "./model.stl"

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

	log.Printf("Received request body (first 100 chars): %s", truncateString(string(bodyBytes), 100))

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

	log.Printf("Successfully parsed request: ModelCode (first 50 chars)=%s", truncateString(req.ModelCode, 50))

	absModelPath, err := filepath.Abs(ModelPath)
	if err != nil {
		log.Printf("Error getting absolute path: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	_ = os.Remove(absModelPath)

	model_expression, err := create_stl(req.ModelCode, absModelPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error parsing templates: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Python expression generated (first 100 chars): %s", truncateString(model_expression, 100))

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
	if err != nil {
		log.Printf("Command execution failed: %v, Output: %s", err, string(output))
		http.Error(w, fmt.Sprintf("Command execution failed: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Blender command executed: output (first 200 chars): %s", truncateString(string(output), 200))

	if _, err := os.Stat(absModelPath); os.IsNotExist(err) {
		log.Printf("Error: STL file was not created at %s", absModelPath)
		http.Error(w, "Failed to generate STL file", http.StatusInternalServerError)
		return
	}

	stlData, err := os.ReadFile(absModelPath)
	if err != nil {
		log.Printf("Error reading STL file: %v", err)
		http.Error(w, "Error reading generated STL file", http.StatusInternalServerError)
		return
	}

	log.Printf("STL file read successfully, size: %d bytes", len(stlData))

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filepath.Base(ModelPath)))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(stlData)))

	if _, err := w.Write(stlData); err != nil {
		log.Printf("Failed to write response: %v", err)
	} else {
		log.Printf("STL file sent successfully")
	}
}

type ModelTemplateVars struct {
	ModelCode string
	Filename  string
}

func create_stl(model_code string, filename string) (string, error) {
	t := template.New("main.py.tmpl")

	t, err := t.ParseFiles("main.py.tmpl")
	if err != nil {
		fmt.Println("Error parsing templates:", err)
		return "", err
	}

	var buf bytes.Buffer
	data := ModelTemplateVars{
		ModelCode: model_code,
		Filename:  filename,
	}

	err = t.ExecuteTemplate(&buf, "main.py.tmpl", data)
	if err != nil {
		fmt.Println("Error executing template:", err)
		return "", err
	}

	result := buf.String()
	return result, nil
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
