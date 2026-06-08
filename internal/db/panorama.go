package db

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/paulmach/orb"
	"github.com/paulmach/orb/encoding/ewkb"

	"github.com/michaelpeterswa/bigpictures.company/internal/problems"
)

// Panorama mirrors a row of the panoramas table.
//
// Location is optional: a nil pointer means the row has no recorded GPS
// location. CapturedAt is similarly optional and uses pgtype.Timestamptz to
// distinguish missing-from-zero-value.
type Panorama struct {
	ID           string
	Slug         string
	Title        string
	Description  string
	CapturedAt   pgtype.Timestamptz
	Location     *orb.Point
	Width        int
	Height       int
	TilePath     string
	ThumbPrefix  string
	OriginalPath string
	EXIF         []byte
	Tags         []string
	CreatedAt    time.Time
}

// querier is the subset of pgx connections this package's INSERT path needs.
// Both *pgxpool.Pool and *pgx.Conn satisfy it; pulling the contract up here
// lets DB and Conn share the SQL.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// InsertPanorama persists a new row, returning the database-assigned ID.
func (d *DB) InsertPanorama(ctx context.Context, p *Panorama) (string, error) {
	return insertPanoramaWith(ctx, d.pool, p)
}

// insertPanoramaWith is the SQL-bearing implementation shared between *DB
// (pool-backed) and *Conn (single-connection, used by the end-of-job INSERT
// path that avoids pgxpool's detached-context dial behavior).
func insertPanoramaWith(ctx context.Context, q querier, p *Panorama) (string, error) {
	const sql = `
		insert into panoramas
			(slug, title, description, captured_at, location, width, height,
			 tile_path, thumb_prefix, original_path, exif, tags)
		values
			($1, $2, nullif($3,''), $4, $5, $6, $7, $8, $9, $10, $11, $12)
		returning id
	`
	locArg, err := encodeLocation(p.Location)
	if err != nil {
		return "", problems.New(
			"db/query-failed",
			"Could not encode location",
			err.Error(),
		)
	}
	var id string
	row := q.QueryRow(
		ctx, sql,
		p.Slug, p.Title, p.Description, p.CapturedAt, locArg,
		p.Width, p.Height, p.TilePath, p.ThumbPrefix, p.OriginalPath,
		nullableJSON(p.EXIF), p.Tags,
	)
	if err := row.Scan(&id); err != nil {
		return "", wrapInsertErr(err, p.Slug)
	}
	return id, nil
}

// GetPanoramaBySlug returns a single panorama by its slug. Returns a
// "db/not-found" Problem if the row does not exist.
func (d *DB) GetPanoramaBySlug(ctx context.Context, slug string) (*Panorama, error) {
	const q = `
		select id, slug, title, coalesce(description,''),
		       captured_at,
		       case when location is null then null
		            else encode(location::bytea, 'hex') end,
		       width, height, tile_path, thumb_prefix, original_path,
		       exif, tags, created_at
		from panoramas
		where slug = $1
	`
	var (
		out     Panorama
		locHex  *string
		exifRaw []byte
	)
	row := d.pool.QueryRow(ctx, q, slug)
	if err := row.Scan(
		&out.ID, &out.Slug, &out.Title, &out.Description,
		&out.CapturedAt, &locHex,
		&out.Width, &out.Height,
		&out.TilePath, &out.ThumbPrefix, &out.OriginalPath,
		&exifRaw, &out.Tags, &out.CreatedAt,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, problems.New(
				NotFoundType[len(problemBase)+1:], // "db/not-found"
				"Panorama not found",
				fmt.Sprintf("no panorama with slug %q", slug),
				problems.WithExt(problems.Ext("slug", slug)),
			)
		}
		return nil, problems.New("db/query-failed", "Could not load panorama", err.Error())
	}
	out.EXIF = exifRaw
	if locHex != nil {
		pt, err := decodeLocationHex(*locHex)
		if err != nil {
			return nil, problems.New("db/query-failed", "Could not decode location", err.Error())
		}
		out.Location = pt
	}
	return &out, nil
}

// ListPanoramas returns all panoramas ordered by captured_at desc (nulls last).
func (d *DB) ListPanoramas(ctx context.Context) ([]Panorama, error) {
	const q = `
		select id, slug, title, coalesce(description,''),
		       captured_at,
		       width, height, tile_path, thumb_prefix, original_path,
		       tags, created_at
		from panoramas
		order by captured_at desc nulls last, created_at desc
	`
	rows, err := d.pool.Query(ctx, q)
	if err != nil {
		return nil, problems.New("db/query-failed", "Could not list panoramas", err.Error())
	}
	defer rows.Close()

	var out []Panorama
	for rows.Next() {
		var p Panorama
		if err := rows.Scan(
			&p.ID, &p.Slug, &p.Title, &p.Description,
			&p.CapturedAt,
			&p.Width, &p.Height,
			&p.TilePath, &p.ThumbPrefix, &p.OriginalPath,
			&p.Tags, &p.CreatedAt,
		); err != nil {
			return nil, problems.New("db/query-failed", "Could not scan panorama row", err.Error())
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, problems.New("db/query-failed", "Row iteration error", err.Error())
	}
	return out, nil
}

// DeletePanoramaBySlug removes a row. Returns "db/not-found" if no row exists.
func (d *DB) DeletePanoramaBySlug(ctx context.Context, slug string) error {
	const q = `delete from panoramas where slug = $1`
	tag, err := d.pool.Exec(ctx, q, slug)
	if err != nil {
		return problems.New("db/query-failed", "Could not delete panorama", err.Error())
	}
	if tag.RowsAffected() == 0 {
		return problems.New(
			NotFoundType[len(problemBase)+1:],
			"Panorama not found",
			fmt.Sprintf("no panorama with slug %q", slug),
			problems.WithExt(problems.Ext("slug", slug)),
		)
	}
	return nil
}

// encodeLocation converts an orb.Point to an EWKB-encoded byte slice with
// SRID 4326. Returns nil for nil input so the SQL writes NULL.
func encodeLocation(p *orb.Point) (any, error) {
	if p == nil {
		return nil, nil
	}
	return ewkb.Value(*p, 4326), nil
}

// decodeLocationHex decodes a hex-encoded EWKB point back to *orb.Point.
func decodeLocationHex(h string) (*orb.Point, error) {
	raw, err := hex.DecodeString(h)
	if err != nil {
		return nil, err
	}
	var pt orb.Point
	scanner := ewkb.Scanner(&pt)
	if err := scanner.Scan(raw); err != nil {
		return nil, err
	}
	return &pt, nil
}

func nullableJSON(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}

func wrapInsertErr(err error, slug string) error {
	// Unique violations on slug surface as a typed slug-conflict problem so
	// callers (and the CLI) can render a useful message without parsing
	// pg error codes themselves.
	if isUniqueViolation(err) {
		return problems.New(
			"db/slug-conflict",
			"Slug already exists",
			fmt.Sprintf("panorama with slug %q already exists", slug),
			problems.WithExt(problems.Ext("slug", slug)),
		)
	}
	return problems.New("db/query-failed", "Could not insert panorama", err.Error())
}

func isUniqueViolation(err error) bool {
	type pgErr interface {
		SQLState() string
	}
	var p pgErr
	if errors.As(err, &p) {
		return p.SQLState() == "23505"
	}
	return false
}
