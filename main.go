package main

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"path/filepath"
	"strings"
	"os/exec"

	"github.com/ncruces/zenity"
	"github.com/zserge/lorca"
)

// Helper to base64 the svg exactly like the HTML report
func loadLogoB64() string {
	svgBytes, err := ioutil.ReadFile("fragMount_white_nobg.svg")
	if err != nil {
		fmt.Println("Warning: Could not load fragMount_white_nobg.svg for the GUI.")
		return ""
	}
	return base64.StdEncoding.EncodeToString(svgBytes)
}

func main() {
	// HTML and CSS for the True Black UI styling
	htmlUI := fmt.Sprintf(`
	<!DOCTYPE html>
	<html>
	<head>
		<title>fMT Demo Manager</title>
		<style>
			body {
				font-family: 'Inter', sans-serif;
				background-color: #000;
				color: #fff;
				margin: 0;
				padding: 2rem;
				display: flex;
				flex-direction: column;
				align-items: center;
				height: 100vh;
				box-sizing: border-box;
				overflow: hidden;
				gap: 1rem;
			}
			.header {
				display: flex;
				align-items: center;
				gap: 15px;
				margin-bottom: 2rem;
			}
			.header h1 {
				font-size: 1.5rem;
				margin: 0;
				font-weight: 600;
			}
			.actions {
				display: flex;
				gap: 1rem;
				width: 100%%;
				max-width: 400px;
				margin-bottom: 2rem;
			}
			button {
				flex: 1;
				background: #fff;
				color: #000;
				border: none;
				padding: 0.8rem 1.2rem;
				font-size: 0.95rem;
				font-weight: 600;
				border-radius: 6px;
				cursor: pointer;
				transition: 0.2s all;
			}
			button:hover {
				background: #ccc;
				transform: translateY(-1px);
			}
			button:active {
				transform: translateY(1px);
			}
			.preview-container {
				width: 100%%;
				max-width: 700px;
				background: #000;
				border: 1px solid #fff;
				border-radius: 6px;
				overflow: hidden;
				display: flex;
				flex-direction: column;
			}
			.preview-header {
				background: #111;
				padding: 0.5rem 1rem;
				border-bottom: 1px solid #333;
				display: flex;
				justify-content: space-between;
				align-items: center;
			}
			.preview-header span {
				font-size: 0.8rem;
				font-weight: bold;
				text-transform: uppercase;
				color: #888;
			}
			iframe {
				border: none;
				width: 100%%;
				height: 450px;
				background: #000;
			}
			.console {
				width: 100%%;
				max-width: 700px;
				height: 120px;
				background: #0a0a0a;
				border: 1px solid #333;
				border-radius: 6px;
				padding: 1rem;
				overflow-y: auto;
				font-family: monospace;
				font-size: 0.85rem;
				color: #a3a3a3;
				white-space: pre-wrap;
			}
			.console span.success {
				color: #10b981;
			}
			.console span.error {
				color: #ef4444;
			}
		</style>
	</head>
	<body>
		<div class="header">
			<img src="data:image/svg+xml;base64,%s" style="width: 50px; height: 50px;" alt="Logo">
			<h1>FMTV Demo Manager</h1>
		</div>
		
		<div class="actions">
			<button onclick="selectFile()">Load Single Demo</button>
			<button onclick="selectDirectory()">Process Folder</button>
		</div>

		<div class="preview-container" id="previewBox" style="display: none;">
			<div class="preview-header">
				<span>Live Preview</span>
				<button onclick="openFullReport()" style="flex: none; padding: 4px 10px; font-size: 0.75rem;">View Full Report</button>
			</div>
			<iframe id="reportFrame"></iframe>
		</div>
		
		<div class="console" id="logBox">
			Waiting for input...
		</div>

		<script>
			// Helper to write to HTML log block
			function logMsg(msg, status = "normal") {
				const lb = document.getElementById("logBox");
				let styledEntry = msg;
				if (status === "success") {
					styledEntry = "<span class='success'>" + msg + "</span>";
				} else if (status === "error") {
					styledEntry = "<span class='error'>" + msg + "</span>";
				}
				lb.innerHTML += "\n" + styledEntry;
				lb.scrollTop = lb.scrollHeight;
			}

			let currentReportPath = "";

			function updatePreview(path) {
				const box = document.getElementById("previewBox");
				const frame = document.getElementById("reportFrame");
				currentReportPath = path;
				
				// Use a cache-buster query param so the iframe reloads even if name is the same
				frame.src = "file:///" + path.replace(/\\/g, '/') + "?t=" + Date.now();
				box.style.display = "flex";
			}

			function openFullReport() {
				if (currentReportPath) {
					nativeOpenInBrowser(currentReportPath);
				}
			}

			// Hook into native Go functions bound by Lorca
			async function selectFile() {
				logMsg("Opening file browser...");
				let result = await nativeSelectFile();
				if (result.error) {
					logMsg(result.error, "error");
				} else if (result.path) {
					logMsg("Processing " + result.path + "...");
					let report = await nativeProcessDemo(result.path);
					if (report.includes("Error")) {
						logMsg(report, "error");
					} else {
						logMsg(report, "success");
						updatePreview(result.path + ".report.html");
					}
				} else {
					logMsg("Selection cancelled.");
				}
			}

			async function selectDirectory() {
				logMsg("Opening folder browser...");
				let result = await nativeSelectDirectory();
				if (result.error) {
					logMsg(result.error, "error");
				} else if (result.files && result.files.length > 0) {
					logMsg("Found " + result.files.length + " demo(s). Starting batch process...");
					let successes = 0;
					for (let i = 0; i < result.files.length; i++) {
						let file = result.files[i];
						logMsg("[" + (i+1) + "/" + result.files.length + "] Processing " + file + "...");
						let report = await nativeProcessDemo(result.files[i]);
						if (report.includes("Error")) {
							logMsg("  -> " + report, "error");
						} else {
							logMsg("  -> " + report, "success");
							successes++;
							updatePreview(file + ".report.html");
						}
					}
					logMsg("Batch complete! Successfully processed " + successes + " demos.", "success");
				} else if (result.files && result.files.length === 0) {
					logMsg("No .dem files found in selected directory.", "error");
				} else {
					logMsg("Selection cancelled.");
				}
			}
		</script>
	</body>
	</html>
	`, loadLogoB64())

	// Write HTML to temporary file to completely side-step windows command-line size limits and Chrome IPC limits
	tempFile := "ui_temp.html"
	err := ioutil.WriteFile(tempFile, []byte(htmlUI), 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(tempFile)
	
	absPath, err := filepath.Abs(tempFile)
	if err != nil {
		log.Fatal(err)
	}
	fileUrl := "file:///" + filepath.ToSlash(absPath)

	// Init Lorca Window (800x850 expanded for preview)
	ui, err := lorca.New(fileUrl, "", 800, 850, "--remote-allow-origins=*")
	if err != nil {
		log.Fatal(err)
	}
	defer ui.Close()

	// Bind native Go SelectFile function to Javascript globally
	ui.Bind("nativeSelectFile", func() map[string]interface{} {
		filename, err := zenity.SelectFile(
			zenity.Title("Select CSGO Demo"),
			zenity.FileFilters{{Name: "CSGO Demo", Patterns: []string{"*.dem"}}},
		)
		if err != nil {
			if err == zenity.ErrCanceled {
				return map[string]interface{}{"path": ""}
			}
			return map[string]interface{}{"error": err.Error()}
		}
		return map[string]interface{}{"path": filename}
	})

	ui.Bind("nativeSelectDirectory", func() map[string]interface{} {
		dir, err := zenity.SelectFile(
			zenity.Title("Select Demo Folder"),
			zenity.Directory(),
		)
		if err != nil {
			if err == zenity.ErrCanceled {
				return map[string]interface{}{"files": nil}
			}
			return map[string]interface{}{"error": err.Error()}
		}

		files, err := ioutil.ReadDir(dir)
		if err != nil {
			return map[string]interface{}{"error": fmt.Sprintf("Failed to read directory: %v", err)}
		}

		var demFiles []string
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(file.Name(), ".dem") {
				demFiles = append(demFiles, filepath.Join(dir, file.Name()))
			}
		}

		return map[string]interface{}{"files": demFiles}
	})

	// Bind demo manager processing engine
	ui.Bind("nativeProcessDemo", func(path string) string {
		return AnalyzeDemo(path) // Directly calls our refactored backend logic
	})

	ui.Bind("nativeOpenInBrowser", func(path string) {
		absPath, _ := filepath.Abs(path)
		// On Windows, 'start' is a cmd builtin, so we call it via cmd /c
		exec.Command("cmd", "/c", "start", absPath).Start()
	})

	// Wait until UI window is safely closed
	<-ui.Done()
}
