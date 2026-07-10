package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/srayansh-gupta/compiling-orchestrator/internal/models"
)

type WorkerRepo struct {
	db *pgxpool.Pool
}

func NewWorkerRepo(db *pgxpool.Pool) *WorkerRepo {
	return &WorkerRepo{db: db}
}

func (r *WorkerRepo) Create(ctx context.Context, w *models.Worker) error {
	q := `INSERT INTO workers (name, ip_address, ssh_username, ssh_password_encrypted, api_key, worker_uuid, provision_status)
		  VALUES ($1, $2, $3, $4, $5, $6, $7)
		  RETURNING id, created_at, updated_at`
	return r.db.QueryRow(ctx, q,
		w.Name, w.IPAddress, w.SSHUsername, w.SSHPasswordEncrypted,
		w.APIKey, w.WorkerUUID, w.ProvisionStatus,
	).Scan(&w.ID, &w.CreatedAt, &w.UpdatedAt)
}

func (r *WorkerRepo) GetByID(ctx context.Context, id string) (*models.Worker, error) {
	q := `SELECT ` + workerCols + ` FROM workers WHERE id = $1`
	row := r.db.QueryRow(ctx, q, id)
	return scanWorker(row)
}

func (r *WorkerRepo) GetByUUID(ctx context.Context, uuid string) (*models.Worker, error) {
	q := `SELECT ` + workerCols + ` FROM workers WHERE worker_uuid = $1`
	row := r.db.QueryRow(ctx, q, uuid)
	return scanWorker(row)
}

func (r *WorkerRepo) GetByAPIKey(ctx context.Context, key string) (*models.Worker, error) {
	q := `SELECT ` + workerCols + ` FROM workers WHERE api_key = $1`
	row := r.db.QueryRow(ctx, q, key)
	return scanWorker(row)
}

func (r *WorkerRepo) List(ctx context.Context) ([]*models.Worker, error) {
	q := `SELECT ` + workerCols + ` FROM workers ORDER BY created_at DESC`
	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var workers []*models.Worker
	for rows.Next() {
		w, err := scanWorker(rows)
		if err != nil {
			return nil, err
		}
		workers = append(workers, w)
	}
	return workers, rows.Err()
}

func (r *WorkerRepo) UpdateStatus(ctx context.Context, id string, status models.ProvisionStatus) error {
	_, err := r.db.Exec(ctx,
		`UPDATE workers SET provision_status = $1, updated_at = NOW() WHERE id = $2`,
		status, id,
	)
	return err
}

func (r *WorkerRepo) UpdateHeartbeat(ctx context.Context, hb *models.Heartbeat) error {
	q := `UPDATE workers SET
			last_heartbeat  = NOW(),
			cpu_usage       = $1,
			ram_usage       = $2,
			disk_usage      = $3,
			network_rx      = $4,
			network_tx      = $5,
			docker_status   = $6,
			nomad_status    = $7,
			judge_status    = $8,
			running_jobs    = $9,
			total_jobs      = $10,
			avg_runtime     = $11,
			uptime          = $12,
			worker_version  = $13,
			hostname        = $14,
			nomad_node_id   = $15,
			provision_status = CASE
				WHEN provision_status NOT IN ('disabled','draining') THEN 'online'
				ELSE provision_status
			END,
			updated_at = NOW()
		  WHERE worker_uuid = $16`
	_, err := r.db.Exec(ctx, q,
		hb.CPUUsage, hb.RAMUsage, hb.DiskUsage,
		hb.NetworkRX, hb.NetworkTX,
		hb.DockerStatus, hb.NomadStatus, hb.JudgeStatus,
		hb.RunningJobs, hb.TotalRequests, hb.AvgRuntime,
		hb.Uptime, hb.WorkerVersion, hb.Hostname, hb.NomadNodeID,
		hb.WorkerUUID,
	)
	return err
}

func (r *WorkerRepo) UpdateSSHKey(ctx context.Context, id, pubKey string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE workers SET ssh_public_key = $1, ssh_password_encrypted = NULL, updated_at = NOW() WHERE id = $2`,
		pubKey, id,
	)
	return err
}

func (r *WorkerRepo) UpdateSystemInfo(ctx context.Context, id string, fields map[string]interface{}) error {
	_, err := r.db.Exec(ctx,
		`UPDATE workers SET
			cpu_model = COALESCE($1, cpu_model),
			cpu_cores = COALESCE($2, cpu_cores),
			total_ram = COALESCE($3, total_ram),
			total_disk = COALESCE($4, total_disk),
			os_version = COALESCE($5, os_version),
			kernel_version = COALESCE($6, kernel_version),
			updated_at = NOW()
		WHERE id = $7`,
		fields["cpu_model"], fields["cpu_cores"], fields["total_ram"],
		fields["total_disk"], fields["os_version"], fields["kernel_version"],
		id,
	)
	return err
}

func (r *WorkerRepo) Delete(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM workers WHERE id = $1`, id)
	return err
}

func (r *WorkerRepo) MarkOfflineStale(ctx context.Context, threshold time.Duration) (int64, error) {
	result, err := r.db.Exec(ctx,
		`UPDATE workers SET provision_status = 'offline', updated_at = NOW()
		 WHERE provision_status = 'online'
		   AND last_heartbeat < NOW() - $1::interval`,
		fmt.Sprintf("%f seconds", threshold.Seconds()),
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected(), nil
}

func (r *WorkerRepo) Stats(ctx context.Context) (map[string]int, error) {
	rows, err := r.db.Query(ctx,
		`SELECT provision_status, COUNT(*) FROM workers GROUP BY provision_status`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	stats := map[string]int{}
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		stats[status] = count
	}
	return stats, rows.Err()
}

func (r *WorkerRepo) AddProvisionLog(ctx context.Context, log *models.ProvisionLog) error {
	q := `INSERT INTO provision_logs (worker_id, step, status, message)
		  VALUES ($1, $2, $3, $4) RETURNING id, created_at`
	return r.db.QueryRow(ctx, q, log.WorkerID, log.Step, log.Status, log.Message).
		Scan(&log.ID, &log.CreatedAt)
}

func (r *WorkerRepo) GetProvisionLogs(ctx context.Context, workerID string) ([]*models.ProvisionLog, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, worker_id, step, status, message, created_at
		 FROM provision_logs WHERE worker_id = $1 ORDER BY created_at ASC`,
		workerID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*models.ProvisionLog
	for rows.Next() {
		l := &models.ProvisionLog{}
		if err := rows.Scan(&l.ID, &l.WorkerID, &l.Step, &l.Status, &l.Message, &l.CreatedAt); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func (r *WorkerRepo) AddAuditLog(ctx context.Context, action, workerID, details string) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO audit_logs (action, worker_id, details) VALUES ($1, NULLIF($2,'')::uuid, $3)`,
		action, workerID, details,
	)
	return err
}

func (r *WorkerRepo) AddNotification(ctx context.Context, n *models.Notification) error {
	q := `INSERT INTO notifications (type, title, message, worker_id)
		  VALUES ($1, $2, $3, NULLIF($4,'')::uuid) RETURNING id, created_at`
	return r.db.QueryRow(ctx, q, n.Type, n.Title, n.Message, n.WorkerID).
		Scan(&n.ID, &n.CreatedAt)
}

func (r *WorkerRepo) GetUnreadNotifications(ctx context.Context) ([]*models.Notification, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id, type, title, message, COALESCE(worker_id::text,''), read, created_at
		 FROM notifications WHERE read = FALSE ORDER BY created_at DESC LIMIT 50`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ns []*models.Notification
	for rows.Next() {
		n := &models.Notification{}
		if err := rows.Scan(&n.ID, &n.Type, &n.Title, &n.Message, &n.WorkerID, &n.Read, &n.CreatedAt); err != nil {
			return nil, err
		}
		ns = append(ns, n)
	}
	return ns, rows.Err()
}

func (r *WorkerRepo) MarkNotificationsRead(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `UPDATE notifications SET read = TRUE WHERE read = FALSE`)
	return err
}

const workerCols = `
	id, name, hostname, ip_address, ssh_username,
	COALESCE(ssh_password_encrypted,''), COALESCE(ssh_public_key,''),
	COALESCE(api_key,''), provision_status, last_heartbeat,
	COALESCE(worker_uuid,''), nomad_node_id,
	cpu_usage, ram_usage, disk_usage, network_rx, network_tx,
	docker_status, nomad_status, judge_status,
	running_jobs, total_jobs, failed_jobs, avg_runtime, uptime,
	worker_version, cpu_model, cpu_cores, total_ram, total_disk,
	os_version, kernel_version, docker_version, nomad_version, judge_version,
	created_at, updated_at`

func scanWorker(row pgx.Row) (*models.Worker, error) {
	w := &models.Worker{}
	err := row.Scan(
		&w.ID, &w.Name, &w.Hostname, &w.IPAddress, &w.SSHUsername,
		&w.SSHPasswordEncrypted, &w.SSHPublicKey,
		&w.APIKey, &w.ProvisionStatus, &w.LastHeartbeat,
		&w.WorkerUUID, &w.NomadNodeID,
		&w.CPUUsage, &w.RAMUsage, &w.DiskUsage, &w.NetworkRX, &w.NetworkTX,
		&w.DockerStatus, &w.NomadStatus, &w.JudgeStatus,
		&w.RunningJobs, &w.TotalJobs, &w.FailedJobs, &w.AvgRuntime, &w.Uptime,
		&w.WorkerVersion, &w.CPUModel, &w.CPUCores, &w.TotalRAM, &w.TotalDisk,
		&w.OSVersion, &w.KernelVersion, &w.DockerVersion, &w.NomadVersion, &w.JudgeVersion,
		&w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return w, nil
}
