package airflowapi

import (
	"context"
	"time"
)

// Assets are Airflow 3's name for what Airflow 2 called datasets. The
// endpoint, the collection key, and several field names all differ, so every
// operation here asks the generation its own way and answers in one shape.

// Asset is a data asset a DAG produces or waits on.
type Asset struct {
	ID  int64  `json:"id"`
	URI string `json:"uri"`
	// Name and Group are Airflow 3 only; Airflow 2 identifies a dataset by
	// its URI alone.
	Name      string    `json:"name"`
	Group     string    `json:"group"`
	Extra     any       `json:"extra"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// ProducingTasks are the tasks that write the asset.
	ProducingTasks []AssetTaskRef `json:"producing_tasks"`
	// ScheduledDAGs are the DAGs the asset triggers, which Airflow 2 sends
	// as consuming_dags.
	ScheduledDAGs []AssetDAGRef `json:"scheduled_dags"`
}

// AssetTaskRef points at a task that produces an asset.
type AssetTaskRef struct {
	DAGID  string `json:"dag_id"`
	TaskID string `json:"task_id"`
}

// AssetDAGRef points at a DAG an asset schedules.
type AssetDAGRef struct {
	DAGID string `json:"dag_id"`
}

// AssetList is a page of assets.
type AssetList struct {
	Assets       []Asset `json:"assets"`
	TotalEntries int     `json:"total_entries"`
}

// AssetEvent is one update of an asset: what produced it, and the runs it
// started.
type AssetEvent struct {
	ID int64 `json:"id"`
	// URI is the asset's URI. Neither generation sends it under this name
	// on an event — Airflow 3 says asset_uri, Airflow 2 dataset_uri.
	URI string `json:"uri"`
	// AssetID is Airflow 3's asset_id, Airflow 2's dataset_id.
	AssetID      int64         `json:"asset_id"`
	Extra        any           `json:"extra"`
	SourceDAGID  string        `json:"source_dag_id"`
	SourceTaskID string        `json:"source_task_id"`
	SourceRunID  string        `json:"source_run_id"`
	Timestamp    time.Time     `json:"timestamp"`
	CreatedDAGs  []AssetDAGRun `json:"created_dagruns"`
}

// AssetDAGRun is a run an asset event started.
type AssetDAGRun struct {
	DAGID    string `json:"dag_id"`
	DAGRunID string `json:"run_id"`
	State    string `json:"state"`
}

// AssetEventList is a page of asset events.
type AssetEventList struct {
	AssetEvents  []AssetEvent `json:"asset_events"`
	TotalEntries int          `json:"total_entries"`
}

// ListAssets lists assets, reading Airflow 2's datasets endpoint when that is
// the generation.
func (c *Client) ListAssets(ctx context.Context, opts ListOptions) (AssetList, error) {
	generation, err := c.Generation(ctx)
	if err != nil {
		return AssetList{}, err
	}
	var wire assetListWire
	if err := c.getCollection(ctx, assetPath(generation, ""), opts.query(), &wire); err != nil {
		return AssetList{}, err
	}
	return wire.list(), nil
}

// ListAssetEventsOptions filters an asset event listing by what produced the
// events.
type ListAssetEventsOptions struct {
	ListOptions
	SourceDAGID  string
	SourceRunID  string
	SourceTaskID string
}

// ListAssetEvents lists asset updates.
func (c *Client) ListAssetEvents(ctx context.Context, opts ListAssetEventsOptions) (AssetEventList, error) {
	generation, err := c.Generation(ctx)
	if err != nil {
		return AssetEventList{}, err
	}
	query := opts.query()
	if opts.SourceDAGID != "" {
		query.Set("source_dag_id", opts.SourceDAGID)
	}
	if opts.SourceRunID != "" {
		query.Set("source_run_id", opts.SourceRunID)
	}
	if opts.SourceTaskID != "" {
		query.Set("source_task_id", opts.SourceTaskID)
	}

	var wire assetEventListWire
	if err := c.getCollection(ctx, assetPath(generation, "/events"), query, &wire); err != nil {
		return AssetEventList{}, err
	}
	return wire.list(), nil
}

// UpstreamAssetEvents lists the asset events that scheduled a run — the
// answer to "why did this run start".
func (c *Client) UpstreamAssetEvents(ctx context.Context, dagID, runID string) (AssetEventList, error) {
	generation, err := c.Generation(ctx)
	if err != nil {
		return AssetEventList{}, err
	}
	tail := "/upstreamAssetEvents"
	if generation == Airflow2 {
		tail = "/upstreamDatasetEvents"
	}
	var wire assetEventListWire
	if err := c.get(ctx, pathf("/dags/%s/dagRuns/%s", dagID, runID)+tail, nil, &wire); err != nil {
		return AssetEventList{}, err
	}
	return wire.list(), nil
}

// assetPath is the collection's name in the generation that serves it.
func assetPath(generation Generation, tail string) string {
	if generation == Airflow2 {
		return "/datasets" + tail
	}
	return "/assets" + tail
}

// The wire types below carry both generations' spellings; the embedded
// exported type supplies the Airflow 3 names and the extra fields the
// Airflow 2 ones.

type assetListWire struct {
	Assets       []assetWire `json:"assets"`
	Datasets     []assetWire `json:"datasets"`
	TotalEntries int         `json:"total_entries"`
}

func (w assetListWire) list() AssetList {
	items := w.Assets
	if len(items) == 0 {
		items = w.Datasets
	}
	list := AssetList{TotalEntries: w.TotalEntries, Assets: make([]Asset, 0, len(items))}
	for i := range items {
		list.Assets = append(list.Assets, items[i].asset())
	}
	return list
}

type assetWire struct {
	Asset
	ConsumingDAGs []AssetDAGRef `json:"consuming_dags"`
}

func (w *assetWire) asset() Asset {
	asset := w.Asset
	if len(asset.ScheduledDAGs) == 0 {
		asset.ScheduledDAGs = w.ConsumingDAGs
	}
	return asset
}

type assetEventListWire struct {
	AssetEvents   []assetEventWire `json:"asset_events"`
	DatasetEvents []assetEventWire `json:"dataset_events"`
	TotalEntries  int              `json:"total_entries"`
}

func (w assetEventListWire) list() AssetEventList {
	items := w.AssetEvents
	if len(items) == 0 {
		items = w.DatasetEvents
	}
	list := AssetEventList{TotalEntries: w.TotalEntries, AssetEvents: make([]AssetEvent, 0, len(items))}
	for i := range items {
		list.AssetEvents = append(list.AssetEvents, items[i].event())
	}
	return list
}

type assetEventWire struct {
	AssetEvent
	AssetURI   string `json:"asset_uri"`
	DatasetURI string `json:"dataset_uri"`
	DatasetID  int64  `json:"dataset_id"`
}

func (w *assetEventWire) event() AssetEvent {
	event := w.AssetEvent
	if event.URI == "" {
		event.URI = w.AssetURI
	}
	if event.URI == "" {
		event.URI = w.DatasetURI
	}
	if event.AssetID == 0 {
		event.AssetID = w.DatasetID
	}
	return event
}
