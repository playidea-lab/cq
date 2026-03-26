//go:build c0_drive

package drive

func init() {
	tusUploadImpl = tusUpload
	resumeDownloadImpl = resumeDownload
}
