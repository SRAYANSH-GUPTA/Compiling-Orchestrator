package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/srayansh-gupta/compiling-orchestrator/internal/models"
	"github.com/srayansh-gupta/compiling-orchestrator/internal/repository"
)

type TestCaseReq struct {
	ID          string  `json:"id"`
	Stdin       string  `json:"stdin"`
	CPULimit    float64 `json:"cpuLimit"`
	MemoryLimit int     `json:"memoryLimit"`
}

type ExecutionRequest struct {
	SubmissionID string        `json:"submissionId"`
	SourceCode   string        `json:"sourceCode"`
	Language     string        `json:"language"`
	Testcases    []TestCaseReq `json:"testcases"`
}

type TestCaseResult struct {
	TestCaseID    string   `json:"testcaseId"`
	Status        string   `json:"status"`
	Stdout        string   `json:"stdout"`
	Stderr        string   `json:"stderr"`
	CompileOutput string   `json:"compileOutput"`
	ExecutionTime float64  `json:"executionTime"`
	WallTime      float64  `json:"wallTime"`
	Memory        int64    `json:"memory"`
	ExitCode      int      `json:"exitCode"`
	Signal        *string  `json:"signal"`
}

type WorkerInfo struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
}

type JudgeInfo struct {
	Version string `json:"version"`
}

type StatusCodeDesc struct {
	Code        int    `json:"code"`
	Description string `json:"description"`
}

type ExecutionResponse struct {
	SubmissionID  string          `json:"submissionId"`
	Status        StatusCodeDesc  `json:"status"`
	Language      string          `json:"language"`
	CompileOutput string          `json:"compileOutput,omitempty"`
	Message       string          `json:"message,omitempty"`
	Results       []TestCaseResult `json:"results"`
	Worker        *WorkerInfo     `json:"worker,omitempty"`
	Judge         *JudgeInfo      `json:"judge,omitempty"`
}

type JudgeGatewayHandler struct {
	repo *repository.WorkerRepo
}

func NewJudgeGatewayHandler(repo *repository.WorkerRepo) *JudgeGatewayHandler {
	return &JudgeGatewayHandler{repo: repo}
}

func (h *JudgeGatewayHandler) Execute(w http.ResponseWriter, r *http.Request) {
	var req ExecutionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, req.SubmissionID, 14, "INTERNAL_ERROR", "Invalid request body: "+err.Error())
		return
	}

	langID := getJudge0LanguageID(req.Language)
	if langID == 0 {
		h.writeError(w, req.SubmissionID, 14, "INTERNAL_ERROR", "Unsupported language: "+req.Language)
		return
	}

	var activeWorker *models.Worker
	var err error

	for attempt := 0; attempt < 3; attempt++ {
		activeWorker, err = h.selectWorker(r.Context())
		if err != nil {
			h.writeError(w, req.SubmissionID, 14, "INTERNAL_ERROR", "No active workers: "+err.Error())
			return
		}

		res, err := h.sendToWorker(r.Context(), activeWorker, req, langID)
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(res)
			return
		}

		h.repo.UpdateStatus(r.Context(), activeWorker.ID, models.StatusOffline)
		h.repo.AddNotification(r.Context(), &models.Notification{
			Type:     "error",
			Title:    "Worker Failure",
			Message:  fmt.Sprintf("Worker %s failed execution request and was marked offline: %v", activeWorker.Name, err),
			WorkerID: activeWorker.ID,
		})
	}

	h.writeError(w, req.SubmissionID, 14, "INTERNAL_ERROR", "All execution attempts failed on available workers.")
}

func (h *JudgeGatewayHandler) selectWorker(ctx context.Context) (*models.Worker, error) {
	workers, err := h.repo.List(ctx)
	if err != nil {
		return nil, err
	}

	var online []*models.Worker
	for _, wk := range workers {
		if wk.ProvisionStatus == models.StatusOnline {
			online = append(online, wk)
		}
	}

	if len(online) == 0 {
		return nil, fmt.Errorf("no workers online")
	}

	sort.Slice(online, func(i, j int) bool {
		if online[i].RunningJobs != online[j].RunningJobs {
			return online[i].RunningJobs < online[j].RunningJobs
		}
		return online[i].CPUUsage < online[j].CPUUsage
	})

	return online[0], nil
}

func (h *JudgeGatewayHandler) sendToWorker(ctx context.Context, wk *models.Worker, req ExecutionRequest, langID int) (*ExecutionResponse, error) {
	type J0Submission struct {
		SourceCode   string  `json:"source_code"`
		LanguageID   int     `json:"language_id"`
		Stdin        string  `json:"stdin,omitempty"`
		CPULimit     float64 `json:"cpu_time_limit,omitempty"`
		MemoryLimit  int     `json:"memory_limit,omitempty"`
	}

	type J0BatchRequest struct {
		Submissions []J0Submission `json:"submissions"`
	}

	var j0Subs []J0Submission
	for _, tc := range req.Testcases {
		stdinB64 := tc.Stdin
		if _, err := base64.StdEncoding.DecodeString(tc.Stdin); err != nil {
			stdinB64 = base64.StdEncoding.EncodeToString([]byte(tc.Stdin))
		}

		j0Subs = append(j0Subs, J0Submission{
			SourceCode:  req.SourceCode,
			LanguageID:  langID,
			Stdin:       stdinB64,
			CPULimit:    tc.CPULimit,
			MemoryLimit: tc.MemoryLimit * 1024,
		})
	}

	j0Req := J0BatchRequest{Submissions: j0Subs}
	jsonBytes, err := json.Marshal(j0Req)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("http://%s:2358/submissions/batch?base64_encoded=true&wait=true", wk.IPAddress)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// Judge0 is configured with AUTHN_TOKEN set to the worker's agent API key,
	// so :2358 is not an unauthenticated code-execution endpoint.
	httpReq.Header.Set("X-Auth-Token", wk.APIKey)

	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("judge0 returned status %d", resp.StatusCode)
	}

	type J0Status struct {
		ID          int    `json:"id"`
		Description string `json:"description"`
	}

	type J0Result struct {
		Stdout        string   `json:"stdout"`
		Stderr        string   `json:"stderr"`
		CompileOutput string   `json:"compile_output"`
		Time          string   `json:"time"`
		Memory        int64    `json:"memory"`
		Status        J0Status `json:"status"`
	}

	var j0Results []J0Result
	if err := json.NewDecoder(resp.Body).Decode(&j0Results); err != nil {
		return nil, err
	}

	var results []TestCaseResult
	hasCompilationError := false
	var firstCompileOutput string

	for i, j0Res := range j0Results {
		tcReq := req.Testcases[i]

		statusVal := "SUCCESS"
		switch j0Res.Status.ID {
		case 3:
			statusVal = "SUCCESS"
		case 4:
			statusVal = "MEMORY_LIMIT_EXCEEDED"
		case 5:
			statusVal = "TIME_LIMIT_EXCEEDED"
		case 6:
			statusVal = "COMPILATION_ERROR"
			hasCompilationError = true
			if firstCompileOutput == "" {
				dec, err := base64.StdEncoding.DecodeString(j0Res.CompileOutput)
				if err == nil {
					firstCompileOutput = string(dec)
				} else {
					firstCompileOutput = j0Res.CompileOutput
				}
			}
		case 13:
			statusVal = "OUTPUT_LIMIT_EXCEEDED"
		case 14:
			statusVal = "INTERNAL_ERROR"
		default:
			statusVal = "RUNTIME_ERROR"
		}

		stdoutDec := ""
		if j0Res.Stdout != "" {
			dec, err := base64.StdEncoding.DecodeString(j0Res.Stdout)
			if err == nil {
				stdoutDec = string(dec)
			} else {
				stdoutDec = j0Res.Stdout
			}
		}

		stderrDec := ""
		if j0Res.Stderr != "" {
			dec, err := base64.StdEncoding.DecodeString(j0Res.Stderr)
			if err == nil {
				stderrDec = string(dec)
			} else {
				stderrDec = j0Res.Stderr
			}
		}

		compOutDec := ""
		if j0Res.CompileOutput != "" {
			dec, err := base64.StdEncoding.DecodeString(j0Res.CompileOutput)
			if err == nil {
				compOutDec = string(dec)
			} else {
				compOutDec = j0Res.CompileOutput
			}
		}

		var execTime float64
		fmt.Sscanf(j0Res.Time, "%f", &execTime)

		results = append(results, TestCaseResult{
			TestCaseID:    tcReq.ID,
			Status:        statusVal,
			Stdout:        stdoutDec,
			Stderr:        stderrDec,
			CompileOutput: compOutDec,
			ExecutionTime: execTime,
			WallTime:      execTime,
			Memory:        j0Res.Memory,
			ExitCode:      0,
		})
	}

	rootCode := 3
	rootDesc := "Success"
	if hasCompilationError {
		rootCode = 6
		rootDesc = "Compilation Error"
	}

	return &ExecutionResponse{
		SubmissionID: req.SubmissionID,
		Status: StatusCodeDesc{
			Code:        rootCode,
			Description: rootDesc,
		},
		Language:      req.Language,
		CompileOutput: firstCompileOutput,
		Results:       results,
		Worker: &WorkerInfo{
			ID:       wk.ID,
			Hostname: wk.Hostname,
		},
		Judge: &JudgeInfo{
			Version: "1.13.0",
		},
	}, nil
}

func (h *JudgeGatewayHandler) writeError(w http.ResponseWriter, subID string, code int, status string, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(ExecutionResponse{
		SubmissionID: subID,
		Status: StatusCodeDesc{
			Code:        code,
			Description: status,
		},
		Message: message,
		Results: []TestCaseResult{},
	})
}

func getJudge0LanguageID(lang string) int {
	switch lang {
	case "c":
		return 50
	case "cpp", "c++":
		return 54
	case "go", "golang":
		return 60
	case "java":
		return 62
	case "javascript", "js":
		return 63
	case "python", "py", "python3":
		return 71
	case "rust":
		return 73
	}
	return 0
}
