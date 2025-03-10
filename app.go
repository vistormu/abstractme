package main

import (
    "encoding/json"
	"net/http"
    "html/template"
	"os"
    "os/exec"
	"path/filepath"
	"regexp"
	"strings"
    "sync"
    "syscall"
    "log"
    "os/signal"
    "embed"

	"github.com/gin-gonic/gin"
)

var (
	currentCmd *exec.Cmd
	cmdLock sync.Mutex
)

//go:embed static/*
var staticFS embed.FS

//go:embed templates/*
var templatesFS embed.FS


const (
    VERSION = "0.0.3"

    STATIC_DIR = "static"
    TEMPLATES_DIR = "templates"
    CONFIGS_DIR = "configs"
    COMMANDS_FILE = "makefile"
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
    args := os.Args[1:]
    if len(args) != 1 {
        log.Fatalf("usage: %s <host:port>\n", os.Args[0])
    }

    hostPort := args[0]

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

    router.StaticFS("/static", http.FS(staticFS))

	// Parse templates from the embedded filesystem.
	tmpl := template.Must(template.ParseFS(templatesFS, "templates/*.html"))
    router.SetHTMLTemplate(tmpl)
    
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
        filename := c.Query("filename")
        content := ""

        if filename == "" {
            c.JSON(http.StatusBadRequest, gin.H{"error": "filename is required"})
            return
        }

        if !strings.HasSuffix(filename, ".toml") {
            filename += ".toml"
        }

        path := filepath.Join(CONFIGS_DIR, filename)
        contentBytes, err := os.ReadFile(path)
        if err != nil {
            c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
            return
        }

        content = string(contentBytes)

        response := map[string]string{
            "filename": strings.TrimSuffix(filename, ".toml"),
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
                filenames = append(filenames, strings.TrimSuffix(file.Name(), ".toml"))
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

        c.Redirect(http.StatusSeeOther, "/files/get")
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
        recipe := c.Query("command")
        if recipe == "" {
            c.HTML(http.StatusBadRequest, "status.html", gin.H{
                "Status": "error",
                "Message": "no command provided",
            })
            return
        }

        cmdLock.Lock()
        defer cmdLock.Unlock()

        if currentCmd != nil {
            c.HTML(http.StatusBadRequest, "status.html", gin.H{
                "Status": "error",
                "Message": "command is already running",
            })
            return
        }

        currentCmd = exec.Command("make", recipe)
        currentCmd.Stdout = os.Stdout
        currentCmd.Stderr = os.Stderr

        currentCmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

        err := currentCmd.Start()
        if err != nil {
            c.HTML(http.StatusInternalServerError, "status.html", gin.H{
                "Status": "error",
                "Message": err.Error(),
            })
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

        c.HTML(http.StatusOK, "status.html", gin.H{
            "Status": "success",
            "Message": "command started: " + recipe,
        })
	})

	router.POST("/commands/stop", func(c *gin.Context) {
        cmdLock.Lock()
        defer cmdLock.Unlock()

        if currentCmd == nil {
            c.HTML(http.StatusBadRequest, "status.html", gin.H{
                "Status": "error",
                "Message": "no command is running",
            })
            return
        }

        err := syscall.Kill(-currentCmd.Process.Pid, syscall.SIGINT)
        if err != nil {
            c.HTML(http.StatusInternalServerError, "status.html", gin.H{
                "Status": "error",
                "Message": err.Error(),
            })
            return
        }

        currentCmd = nil

        c.HTML(http.StatusOK, "status.html", gin.H{
            "Status": "error",
            "Message": "command stopped",
        })
	})

    // ======
    // themes
    // ======
    router.GET("/themes/get", func(c *gin.Context) {
        c.HTML(http.StatusOK, "themes.html", gin.H{"Themes": themes})
    })

    // =======
    // updates
    // =======
    router.GET("/update/check", func(c *gin.Context) {
        apiURL := "https://api.github.com/repos/vistormu/abstractme/releases/latest"
        resp, err := http.Get(apiURL)
        if err != nil {
            log.Printf("failed to query GitHub: %s\n", err.Error())
            c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to query GitHub: " + err.Error()})
            return
        }
        defer resp.Body.Close()

        if resp.StatusCode != http.StatusOK {
            log.Printf("unexpected GitHub response: %s\n", resp.Status)
            c.JSON(http.StatusInternalServerError, gin.H{"error": "unexpected GitHub response: " + resp.Status})
            return
        }

        // Define a struct for parsing the JSON response
        var release struct {
            TagName     string `json:"tag_name"`
            Body        string `json:"body"`
            PublishedAt string `json:"published_at"`
        }

        if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
            log.Printf("failed to decode GitHub response: %s\n", err.Error())
            c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to decode GitHub response: " + err.Error()})
            return
        }

        // Remove any "v" prefix from the tag (if present) so that "v0.1.0" and "0.1.0" are considered the same.
        latestVersion := strings.TrimPrefix(release.TagName, "v")
        updateAvailable := latestVersion != VERSION

        c.HTML(http.StatusOK, "update-button.html", gin.H{
            "UpdateAvailable": updateAvailable,
        })
    })
    
    gin.SetMode(gin.ReleaseMode)
    router.Run(hostPort)
}
