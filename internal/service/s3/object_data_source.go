// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package s3

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/YakDriver/regexache"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-provider-aws/internal/conns"
	"github.com/hashicorp/terraform-provider-aws/internal/errs/sdkdiag"
	"github.com/hashicorp/terraform-provider-aws/internal/flex"
	tftags "github.com/hashicorp/terraform-provider-aws/internal/tags"
)

// @SDKDataSource("aws_s3_object")
func DataSourceObject() *schema.Resource {
	return &schema.Resource{
		ReadWithoutTimeout: dataSourceObjectRead,

		Schema: map[string]*schema.Schema{
			"body": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"bucket": {
				Type:     schema.TypeString,
				Required: true,
			},
			"bucket_key_enabled": {
				Type:     schema.TypeBool,
				Computed: true,
			},
			"cache_control": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"content_disposition": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"content_encoding": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"content_language": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"content_length": {
				Type:     schema.TypeInt,
				Computed: true,
			},
			"content_type": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"etag": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"expiration": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"expires": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"key": {
				Type:     schema.TypeString,
				Required: true,
			},
			"last_modified": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"metadata": {
				Type:     schema.TypeMap,
				Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},
			"object_lock_legal_hold_status": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"object_lock_mode": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"object_lock_retain_until_date": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"range": {
				Type:     schema.TypeString,
				Optional: true,
			},
			"server_side_encryption": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"sse_kms_key_id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"storage_class": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"version_id": {
				Type:     schema.TypeString,
				Optional: true,
				Computed: true,
			},
			"website_redirect_location": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"tags": tftags.TagsSchemaComputed(),
		},
	}
}

func dataSourceObjectRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {
	var diags diag.Diagnostics
	conn := meta.(*conns.AWSClient).S3Conn(ctx)
	ignoreTagsConfig := meta.(*conns.AWSClient).IgnoreTagsConfig

	bucket := d.Get("bucket").(string)
	key := d.Get("key").(string)

	input := s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}
	if v, ok := d.GetOk("range"); ok {
		input.Range = aws.String(v.(string))
	}
	if v, ok := d.GetOk("version_id"); ok {
		input.VersionId = aws.String(v.(string))
	}

	versionText := ""
	uniqueId := bucket + "/" + key
	if v, ok := d.GetOk("version_id"); ok {
		versionText = fmt.Sprintf(" of version %q", v.(string))
		uniqueId += "@" + v.(string)
	}

	log.Printf("[DEBUG] Reading S3 Object: %s", input)
	out, err := conn.HeadObjectWithContext(ctx, &input)
	if err != nil {
		return sdkdiag.AppendErrorf(diags, "getting S3 Bucket (%s) Object (%s): %s", bucket, key, err)
	}
	if aws.BoolValue(out.DeleteMarker) {
		return sdkdiag.AppendErrorf(diags, "Requested S3 object %q%s has been deleted", bucket+key, versionText)
	}

	log.Printf("[DEBUG] Received S3 object: %s", out)

	d.SetId(uniqueId)

	d.Set("bucket_key_enabled", out.BucketKeyEnabled)
	d.Set("cache_control", out.CacheControl)
	d.Set("content_disposition", out.ContentDisposition)
	d.Set("content_encoding", out.ContentEncoding)
	d.Set("content_language", out.ContentLanguage)
	d.Set("content_length", out.ContentLength)
	d.Set("content_type", out.ContentType)
	// See https://forums.aws.amazon.com/thread.jspa?threadID=44003
	d.Set("etag", strings.Trim(aws.StringValue(out.ETag), `"`))
	d.Set("expiration", out.Expiration)
	d.Set("expires", out.Expires)
	if out.LastModified != nil {
		d.Set("last_modified", out.LastModified.Format(time.RFC1123))
	} else {
		d.Set("last_modified", "")
	}
	d.Set("metadata", flex.PointersMapToStringList(out.Metadata))
	d.Set("object_lock_legal_hold_status", out.ObjectLockLegalHoldStatus)
	d.Set("object_lock_mode", out.ObjectLockMode)
	d.Set("object_lock_retain_until_date", flattenObjectDate(out.ObjectLockRetainUntilDate))
	d.Set("server_side_encryption", out.ServerSideEncryption)
	d.Set("sse_kms_key_id", out.SSEKMSKeyId)
	d.Set("version_id", out.VersionId)
	d.Set("website_redirect_location", out.WebsiteRedirectLocation)

	// The "STANDARD" (which is also the default) storage
	// class when set would not be included in the results.
	if out.StorageClass == nil {
		d.Set("storage_class", s3.StorageClassStandard)
	} else {
		d.Set("storage_class", out.StorageClass)
	}

	if isContentTypeAllowed(out.ContentType) {
		input := s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(key),
		}
		if v, ok := d.GetOk("range"); ok {
			input.Range = aws.String(v.(string))
		}
		if out.VersionId != nil {
			input.VersionId = out.VersionId
		}
		out, err := conn.GetObjectWithContext(ctx, &input)
		if err != nil {
			return sdkdiag.AppendErrorf(diags, "Failed getting S3 object: %s", err)
		}

		buf := new(bytes.Buffer)
		bytesRead, err := buf.ReadFrom(out.Body)
		if err != nil {
			return sdkdiag.AppendErrorf(diags, "Failed reading content of S3 object (%s): %s", uniqueId, err)
		}
		log.Printf("[INFO] Saving %d bytes from S3 object %s", bytesRead, uniqueId)
		d.Set("body", buf.String())
	} else {
		contentType := ""
		if out.ContentType == nil {
			contentType = "<EMPTY>"
		} else {
			contentType = aws.StringValue(out.ContentType)
		}

		log.Printf("[INFO] Ignoring body of S3 object %s with Content-Type %q", uniqueId, contentType)
	}

	tags, err := ObjectListTagsV1(ctx, conn, bucket, key)

	if err != nil {
		return sdkdiag.AppendErrorf(diags, "listing tags for S3 Bucket (%s) Object (%s): %s", bucket, key, err)
	}

	if err := d.Set("tags", tags.IgnoreAWS().IgnoreConfig(ignoreTagsConfig).Map()); err != nil {
		return sdkdiag.AppendErrorf(diags, "setting tags: %s", err)
	}

	return diags
}

// This is to prevent potential issues w/ binary files
// and generally unprintable characters
// See https://github.com/hashicorp/terraform/pull/3858#issuecomment-156856738
func isContentTypeAllowed(contentType *string) bool {
	if contentType == nil {
		return false
	}

	allowedContentTypes := []*regexp.Regexp{
		regexache.MustCompile(`^application/atom\+xml$`),
		regexache.MustCompile(`^application/json$`),
		regexache.MustCompile(`^application/ld\+json$`),
		regexache.MustCompile(`^application/x-csh$`),
		regexache.MustCompile(`^application/x-httpd-php$`),
		regexache.MustCompile(`^application/x-sh$`),
		regexache.MustCompile(`^application/xhtml\+xml$`),
		regexache.MustCompile(`^application/xml$`),
		regexache.MustCompile(`^text/.+`),
	}

	for _, r := range allowedContentTypes {
		if r.MatchString(*contentType) {
			return true
		}
	}

	return false
}
