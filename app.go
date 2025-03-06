package main

import (
	"net/http"
	"os"
    "os/exec"
	"path/filepath"
	"regexp"
	"strings"
    "sync"
    "syscall"
    "log"
    "os/signal"

	"github.com/gin-gonic/gin"
)

var (
	currentCmd *exec.Cmd
	cmdLock sync.Mutex
)


const (
    STATIC_DIR = "static"
    TEMPLATES_DIR = "templates"
    // CONFIGS_DIR = "configs"
    // COMMANDS_FILE = "makefile"
    CONFIGS_DIR = "../configs"
    COMMANDS_FILE = "../makefile"
)

var themes = []string{
    "light",
    "nord",
    "cupcake",
    "bumblebee",
    "emerald",
    
    "dark",
    "black",
    "dracula",
    "business",
    "dim",

    "cyberpunk",
    "retro",
    "lemonade",
    "caramellatte",
    "valentine",
}

func main() {
    sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)

	go func() {
		<-sigChan
		cmdLock.Lock()
		if currentCmd != nil && currentCmd.Process != nil {
			syscall.Kill(-currentCmd.Process.Pid, syscall.SIGINT)
		}
		cmdLock.Unlock()
		os.Exit(0)
	}()

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
            if strings.HasSuffix(file.Name(), ".toml") {
                filenames = append(filenames, file.Name())
            }
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
        recipe := c.PostForm("command")
        if recipe == "" {
            c.JSON(http.StatusBadRequest, gin.H{"error": "command is required"})
            return
        }

        cmdLock.Lock()
        defer cmdLock.Unlock()

        if currentCmd != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": "command is already running"})
            return
        }

        currentCmd = exec.Command("make", recipe)
        currentCmd.Stdout = os.Stdout
        currentCmd.Stderr = os.Stderr
        currentCmd.Dir = ".."

        currentCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

        err := currentCmd.Start()
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }

        go func(cmd *exec.Cmd) {
            err := currentCmd.Wait()

            cmdLock.Lock()
            defer cmdLock.Unlock()

            currentCmd = nil

            if err != nil {
                log.Printf("command failed: %s\n", err.Error())
                return
            }
        }(currentCmd)

        c.JSON(http.StatusOK, gin.H{"message": "command started: " + recipe})
	})

	router.POST("/commands/stop", func(c *gin.Context) {
        cmdLock.Lock()
        defer cmdLock.Unlock()

        if currentCmd == nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": "no command is running"})
            return
        }

        err := syscall.Kill(-currentCmd.Process.Pid, syscall.SIGINT)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }

        c.JSON(http.StatusOK, gin.H{"message": "command stopped"})
	})

    // ======
    // themes
    // ======
    router.GET("/themes/get", func(c *gin.Context) {
        c.HTML(http.StatusOK, "header.html", gin.H{"Themes": themes})
    })

    router.Run(":8080")
}
