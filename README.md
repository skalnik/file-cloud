# File Cloud

Upload files and get short links.

![](https://cdn.skalnik.com/EuaIWtTPfkf9qijqHUCWRqbQPK4wMxc054CugSiZzh0%2Ffile+cloud+2023-06-23.gif)

## It's beautiful, how do I run it?

1. Step up an S3 bucket in us-west-1 (maybe this should be configurable).
   Optionally, set up a CloudFront distribution for that bucket.
2. `make build` will give you an `app` executable you can deploy where ever.
   Alternatively services like [Render](https://render.com) or
   [Fly.io](https://fly.io) work well.
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

## What the hell did you shove into my S3 bucket and how do these URLs even?!

I wanted to avoid having a databass and minimal additional libraries, so S3 keys
are tied directly to file content, rather than any ID creation. Files are
SHA-256 hashed which is then URL safe base 64 encoded without padding and used
as a prefix for the key. The original file name is then appended to that, as to
retain the original name when downloaded or displayed.

Lookup URLs are then shortened versions of that base 64 encoded hash, and S3
keys are looked up by that prefix. The length of that prefix can be increased if
you're concerned about hash collisions.
