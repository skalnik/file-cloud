# File Cloud

Upload files and get short links.

## To Do

- [x] HTTP works
- [x] Can be [deployed](https://render.com) and work
- [x] Can `POST` a file
- [x] Ugh, some kinda logging or something idk
- [x] Authentication
- [x] Uploads file to S3
- [x] Can display files
- [x] Wire up drag & drop to POST file and then redirect to it
- [x] Rework s3 file names `<full_hash>/og_filename.txt`. Can match subset of
    key prefix on lookup
- [x] Looks better
- [x] No auth required to view
- [x] Images are sized well for browser
- [x] 404 Page
- [x] Testing
- [x] Maybe not One Big File 😬
- [ ] Ok test more critical things lmao
- [x] Links should expand images in like Slack and shit
- [x] Slap CDN in front of S3
- [ ] Make `<key>.ext` redirect to signed URL for direct linking
- [x] Learn wtf `context.TODO` is and what we should be using instead
- [ ] Imma have to make some kinda listing shit, aren't I?
- [x] [macOS Client](https://github.com/skalnik/file-cloud-app)


## It's beautiful, how do I run it?

1. Step up an S3 Bucket in us-west-1 (maybe this should be configurable)
2. `make build` will give you an `app` executable you can deploy where ever
3. Run it with some environment variables (or pass as flags):
  - `PORT`: What port to listen on
  - `KEY`: An AWS key
  - `SECRET`: An AWS secret
  - `BUCKET`: The S3 bucket to store content in
  - `CDN` (Optional): A CDN URL to use with your S3 object keys. If blank, will
      use pre-signed S3 URLs instead.
  - `USERNAME` (Optional): A username to secure uploading behind with basic
      authentication
  - `PASSWORD` (Optional): A password to secure uploading behind with basic
      authentication
  - `PLAUSIBLE` (Optional): A domain to use with
      [Plausible](https://plausible.io/) for metrics
