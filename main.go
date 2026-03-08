package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
			button:disabled {
				background: #333;
				color: #666;
				cursor: not-allowed;
				transform: none;
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

			function setBusy(isBusy) {
				const buttons = document.querySelectorAll("button");
				buttons.forEach(b => b.disabled = isBusy);
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

			function handleFileSelected(path) {
				if (path) {
					logMsg("Processing " + path + "...");
					processOne(path);
				} else {
					logMsg("Selection cancelled.");
				}
				setBusy(false);
			}

			function handleDirectorySelected(filesJson) {
				if (filesJson) {
					const files = JSON.parse(filesJson);
					if (files.length > 0) {
						logMsg("Found " + files.length + " demo(s). Starting batch process...");
						processBatch(files);
					} else {
						logMsg("No .dem files found in selected directory.", "error");
						setBusy(false);
					}
				} else {
					logMsg("Selection cancelled.");
					setBusy(false);
				}
			}

			async function processOne(path) {
				let report = await nativeProcessDemo(path);
				if (report.includes("Error")) {
					logMsg(report, "error");
				} else {
					logMsg(report, "success");
					updatePreview(path + ".report.html");
				}
			}

			async function processBatch(files) {
				let successes = 0;
				for (let i = 0; i < files.length; i++) {
					let file = files[i];
					logMsg("[" + (i+1) + "/" + files.length + "] Processing " + file + "...");
					let report = await nativeProcessDemo(file);
					if (report.includes("Error")) {
						logMsg("  -> " + report, "error");
					} else {
						logMsg("  -> " + report, "success");
						successes++;
						updatePreview(file + ".report.html");
					}
				}
				logMsg("Batch complete! Successfully processed " + successes + " demos.", "success");
				setBusy(false);
			}

			// Hook into native Go functions bound by Lorca
			function selectFile() {
				setBusy(true);
				logMsg("Opening file browser...");
				nativeSelectFile(); // Returns immediately, logic continues in handleFileSelected
			}

			function selectDirectory() {
				setBusy(true);
				logMsg("Opening folder browser...");
				nativeSelectDirectory(); // Returns immediately, logic continues in handleDirectorySelected
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
	// We use goroutines to avoid deadlocking the Lorca message loop
	ui.Bind("nativeSelectFile", func() {
		go func() {
			fmt.Println("[GUI] nativeSelectFile triggered")
			filename, _ := zenity.SelectFile(
				zenity.Title("Select fragMount Demo"),
				zenity.FileFilters{zenity.FileFilter{Name: "Demo File", Patterns: []string{"*.dem"}}},
			)
			fmt.Println("[GUI] nativeSelectFile returned:", filename)
			
			// Escape backslashes for JS string
			safePath := strings.ReplaceAll(filename, "\\", "\\\\")
			ui.Eval(fmt.Sprintf("handleFileSelected('%s')", safePath))
		}()
	})

	ui.Bind("nativeSelectDirectory", func() {
		go func() {
			fmt.Println("[GUI] nativeSelectDirectory triggered")
			dir, _ := zenity.SelectFile(
				zenity.Title("Select Demo Folder"),
				zenity.Directory(),
			)
			fmt.Println("[GUI] nativeSelectDirectory returned:", dir)
			
			if dir == "" {
				ui.Eval("handleDirectorySelected(null)")
				return
			}

			files, _ := ioutil.ReadDir(dir)
			var demFiles []string
			for _, file := range files {
				if !file.IsDir() && strings.HasSuffix(file.Name(), ".dem") {
					demFiles = append(demFiles, filepath.Join(dir, file.Name()))
				}
			}

			// Encode results to JSON to pass safely back to JS
			jsonFiles, _ := json.Marshal(demFiles)
			safeJson := strings.ReplaceAll(string(jsonFiles), "\\", "\\\\")
			ui.Eval(fmt.Sprintf("handleDirectorySelected('%s')", safeJson))
		}()
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
