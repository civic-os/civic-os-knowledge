// Package storage syncs the local bundle and version directories with
// S3-compatible object storage (DigitalOcean Spaces), which is the source of
// truth: pull on startup, push and delete on write.
package storage
