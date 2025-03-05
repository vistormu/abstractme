package main

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)


const (
    STATIC_DIR = "static"
    TEMPLATES_DIR = "templates"
    CONFIGS_DIR = "configs"
    COMMANDS_FILE = "makefile"
)

var (
	currentCmd *exec.Cmd
	currentCmdMutex sync.Mutex
)

func main() {
	router := gin.Default()

    router.Static("/static", STATIC_DIR)
    router.LoadHTMLGlob(filepath.Join(TEMPLATES_DIR, "*"))
    
    // ====
    // root
    // ====
	router.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "base.html", nil)
	})

    // ======
    // editor
    // ======
    router.POST("/editor/update", func(c *gin.Context) {
        filename := c.PostForm("filename")
        content := ""

        if filename != "" {
            path := filepath.Join(CONFIGS_DIR, filename)
            contentBytes, err := os.ReadFile(path)
            if err != nil {
                c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
                return
            }

            content = string(contentBytes)
        }

        response := map[string]string{
            "filename": filename,
            "content": content,
        }

        c.HTML(http.StatusOK, "editor.html", response)
    })

    router.POST("/files/delete", func(c *gin.Context) {
        filename := c.PostForm("filename")
        path := filepath.Join(CONFIGS_DIR, filename)

        err := os.Remove(path)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }
    })

    // =====
    // files
    // =====
    router.GET("/files/get", func(c *gin.Context) {
        files, err := os.ReadDir(CONFIGS_DIR)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})

            return
        }

        filenames := []string{}
        for _, file := range files {
            filenames = append(filenames, file.Name())
        }

        c.HTML(http.StatusOK, "files.html", gin.H{"Files": filenames})
    })

    router.POST("/files/save", func(c *gin.Context) {
        filename := c.PostForm("filename")
        if !strings.HasSuffix(filename, ".toml") {
            filename += ".toml"
        }

        content := c.PostForm("content")

        path := filepath.Join(CONFIGS_DIR, filename)
        err := os.WriteFile(path, []byte(content), 0644)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }

        c.JSON(http.StatusOK, gin.H{"message": "File saved successfully"})
    })


    // ========
    // commands
    // ========
    router.GET("/commands/get", func(c *gin.Context) {
        contentBytes, err := os.ReadFile(COMMANDS_FILE)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }

        content := string(contentBytes)

        // get the recipes from the makefile
        recipeRegex := regexp.MustCompile(`(?m)^[a-zA-Z0-9_-]+:.*`)
        recipes := recipeRegex.FindAllString(content, -1)

        for i, recipe := range recipes {
            recipes[i] = strings.TrimSpace(recipe)
            recipes[i] = strings.ReplaceAll(recipes[i], ":", "")
        }

        c.HTML(http.StatusOK, "commands.html", gin.H{"Commands": recipes})
    })

	router.POST("/commands/run", func(c *gin.Context) {
		currentCmdMutex.Lock()
		defer currentCmdMutex.Unlock()

		// Check if a command is already running.
		if currentCmd != nil {
			c.JSON(http.StatusConflict, gin.H{"error": "A command is already running"})
			return
		}

		// Retrieve the command target from the form.
		commandName := c.PostForm("command")
		if commandName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No command provided"})
			return
		}

		// Create the make command.
		cmd := exec.Command("make", commandName)
		// (Optional) Set output streams, e.g., cmd.Stdout = os.Stdout, cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Save the running command.
		currentCmd = cmd

		// Wait for the command to finish in a separate goroutine.
		go func() {
			err := cmd.Wait()
			if err != nil {
				// Optionally log the error.
			}
			// Reset currentCmd when finished.
			currentCmdMutex.Lock()
			currentCmd = nil
			currentCmdMutex.Unlock()
		}()

		c.JSON(http.StatusOK, gin.H{"message": "Command started"})
	})

	router.POST("/commands/stop", func(c *gin.Context) {
		currentCmdMutex.Lock()
		defer currentCmdMutex.Unlock()

		if currentCmd == nil {
			c.JSON(http.StatusOK, gin.H{"message": "No command is running"})
			return
		}

		// Kill the process.
		if err := currentCmd.Process.Kill(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Reset the command since it has been stopped.
		currentCmd = nil
		c.JSON(http.StatusOK, gin.H{"message": "Command stopped"})
	})

    // start the server on localhost:8080
    router.Run(":8080")
}
