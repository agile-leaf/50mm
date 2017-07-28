# 50mm
50mm (Go package name `fiftymm`) is a HTML image gallery software written in Go. It can serve very minimalistic HTML galleries of your photographs.

You can setup 50mm to serve one album per domain (an example can be seen at [https://baku.50mm.asadjb.com/](https://baku.50mm.asadjb.com/)), or you can setup 50mm to serve multiple albums per domain. An example of the later can be seen at [https://50mm.asadjb.com/](https://50mm.asadjb.com/).

## Why another web image gallery software?
Fair question. There are a couple of great options out there for web image galleries. But we had a very specific list of requirements we wanted, which is why we created 50mm:
- Point it to an S3 bucket and it should just work. While a lot of the options out there allow you to upload your photos to an S3 bucket of your choice, none of them work the other way around. That is, you can't configure them to read images from an S3 bucket. You have to upload them via their web interfaces.
- Images should not need to be uploaded via a web interface. Web interfaces don't provide the best experience when uploading a large number of big sized files. Whereas uploading reliably to S3 is a [solved](https://cyberduck.io/) [problem](https://panic.com/transmit/).
- Shouldn't need to pre-process the images. There's a bunch of different services that offer on-demand image manipulation. The one we wanted to use was [Imgix](https://www.imgix.com/), but the actual service didn't matter. Only that there shouldn't be a pre-processing step.
- Ideally, no database backend required for the images. It should serve whatever it finds in the S3 bucket, without needing to first sync up the list of images with some database. This one is a purely "nice to have". If we had found something that checked our other requirements and used a DB, we would have used that.

## How do I use it?
You'll need a working installation of [Go](https://golang.org/) to build 50mm. At this time, we don't provide prebuilt binaries. You'll also want to have a web server where you can run this.

### Deploying the web application
You can get and build the 50mm software by running:

go get github.com/agile-leaf/50mm

This should produce a binary file named `50mm` inside the `bin` folder in your Go workspace. This is the server component of the application. To keep things organised, let's copy the binary file to a new folder, which I refer to in the rest of this documentation as the `deploy` folder.

Next copy the `templates` and `static` folders from `$GOPATH/src/github.com/agile-leaf/50mm` into the `deploy` folder. Your `deploy` folder should now have the following structure, although the exact files in the `static` and `templates` folders may differ for different versions of the software. What matters is the placement of those folders relative to the binary file `50mm`:

	deploy
	├── 50mm
	├── static
	│   ├── album.css
	│   ├── base.css
	│   ├── echo.min.js
	│   ├── index.css
	│   └── placeholder.png
	└── templates
	├── album.html
	└── index.html

Next we need to create a `config` folder to hold the configuration files for our sites and albums. This folder can be anywhere on your system, but I just create it inside the `deploy` folder to keep things simple.

### Configure a new site and album
Inside the `config` folder, create a new INI file. Call it whatever you want, but it's best to name it after the site domain, as it allows you to easily find it again. For this example, I'll call it `50mm.ini`. Here's the sample config file I use for my site:

	[DEFAULT]
	Domain = 50mm.asadjb.com
	CanonicalSecure = 1
	BucketRegion = eu-west-1
	BucketName = 50mm.photos
	UseImgix = 1
	BaseUrl = https://50mm-photos.imgix.net
	AWSKeyId = AWS_ACCESS_KEY
	AWSKey = AWS_SECRET_KEY
	SiteTitle = 50mm
	MetaTitle = 50mm | Photos by Jibran
	HasAlbumIndex = 1
	
	[Baku]
	Path = /baku/
	BucketPrefix = baku/
	MetaTitle = Baku, Azerbaijan | Photos by Jibran
	AlbumTitle = Baku, Azerbaijan
	
	[Salalah]
	Path = /salalah/
	BucketPrefix = salalah/
	MetaTitle = Salalah, Oman | Photos by Jibran
	AlbumTitle = Salalah, Oman

This configuration is for a site that has an index page, uses Imgix for optimised images, and has two albums. If you want to serve multiple sites, create multiple configuration files. Read on to understand what each of these configuration options mean.

The `[DEFAULT]` section holds configurations for the entire site. Any other section in the config file is parsed as configuration for an album in the site.

#### DEFAULT configuration options
- `Domain`: This is the domain you want to configure your site on. 50mm will serve this site only if the request domain matches this.
- `CanonicalSecure`: The 50mm server doesn't handle SSL connections. To get around this, 50mm is usually deployed behind a proxy server, like nginx. Right now 50mm doesn't look at any headers to tell if the original request was on a secure URL or not. If the `CanonicalSecure` configuration option is set to 1, 50mm assumes all requests are coming from a secure URL, and creates `https` URLs in the HTML it generates.
- `BucketRegion`: The AWS S3 region that hosts your photos bucket.
- `BucketName`: Name of your S3 bucket.
- `UseImgix`: If set to 1, the image URLs generated for your albums will use the Imgix image transformation service. This results in smaller image sizes and a faster web site, but Imgix is a paid service. If you turn this off (by setting the option to 0), the image URLs on your site will be AWS S3 URLs of the files you upload.
- `BaseUrl`: The base URL for your Imgix account. Look at the section _Imgix setup_ below to understand what value to put here. You can skip this option if you don't use Imgix.
- `AWSKeyId`: The AWS access key for an IAM user that has read access to your photos bucket.
- `AWSKey`: The AWS secret key for your IAM user.
- `SiteTitle`: Name of the site, displayed as the `H1` heading on all pages of the site.
- `MetaTitle`: Used as the HTML page title for the home page of your site.
- `HasAlbumIndex`: If set to 1, 50mm will create an index page for the website which lists all public albums (more on public/private albums in the next section). You can set this to 0 if you don't want the index page, for example if you want to keep your list of albums private.
- `AuthUser`: You can use HTTP basic auth to provide simple password protection for your site. This is the username for that. If you don't need auth, skip this option.
- `AuthPass`: The password for HTTP basic auth. Skip this option if you don't want auth.
### Album configuration options
Any section in the INI file other than the `DEFAULT` is considered an album. Here's a list of the configuration options for an album:
- `Path`: The path on which to serve this album. In our example config, the album "Salalah" is served on the URL `50mm.asadjb.com/salalah/`.
- `BucketPrefix`: The prefix (folder) on the S3 bucket that stores the photos for this album. Each album must have a prefix.
- `MetaTitle`: The HTML title for the album page.
- `AlbumTitle`: The title used in the H2 tag on the album page.
- `InIndex`: You can configure individual albums to not show up in the site index. The site index is the home page which lists all your configured albums. True by default. Set to 0 to turn this off.
- `AuthUser`: In addition to having HTTP basic auth site wide, you can configure each album to have it's own authentication username and password. Skip this option if not required.
- `AuthPass`: Password for album specific auth. Skip this option if not required.

There are a few things to remember about using authentication:
 - If your album has `AuthUser` and `AuthPass` set, then `InIndex` can not be true. This is to make sure that any albums you want to keep private don't show their photos on the site index.
- If your album has auth configured, then accessing the album page will use the username and password for that album, wether your site has it's auth configured or not.
- But if your album does not have any auth settings, and the site does, the album will use the username and password you configured for your site. This is another design decision to ensure that if a site is marked as private (by requiring auth), all it's albums are private as well.

You can also have albums served on the site root. So instead of showing a list of albums on the root domain `50mm.asadjb.com`, you can instead just show the album page. To configure this, set the `HasAlbumIndex` in the site config to 0 and set the `Path` for the album you want at the root to `/`.

### Configuring Imgix
You can use the image transformation service Imgix to serve optimised images. To do so, you first need to get an Imgix account, and setup a source to point to the same AWS S3 bucket you have configured for the site.

Once that's done, you can copy the "Imgix Domain" for that source, which looks something like [https://source-name.imgix.net](https://50mm-photos.imgix.net)and use it as the value for `BaseUrl` in your site config.

### Configuring Nginx
If you use Nginx as your reverse proxy in-front of 50mm, you can use a configuration file similar to this:

	server {
	    listen 80;
	    server_name 50mm.asadjb.com;
	
	    location / {
	        proxy_pass http://127.0.0.1:8080;
	        proxy_set_header Host $http_host;
	    }
	}

You can also have SSL setup on Nginx if needed. Just remember to turn on the `CanonicalSecure` setting in your site config.

### Setup the 50mm server
You can use whichever solution you want to keep the 50mm server running in the background. I personally use `supervisord`, but you can use `init`, `upstart`, `systemd`, or any other solution you want; including running it inside a `tmux` session if you feel brave!

Just remember to setup the `FIFTYMM_CONFIG_DIR` and `FIFTYMM_PORT` environment variables.

Here's the `supervisord` config I use:

	[program:50mm]
	command=/home/asadjb/webapps/50mm/fiftymm
	directory=/home/asadjb/webapps/50mm
	environment=FIFTYMM_CONFIG_DIR="/home/asadjb/webapps/50mm/config",FIFTYMM_PORT="12536"
	stdout_logfile=/home/asadjb/logs/user/50mm_stdout.log
	stderr_logfile=/home/asadjb/logs/user/50mm_stderr.log

## Upload photos and bask in the glory!
Once the web app is up and running, you can upload photos to your S3 bucket (inside the folders/prefixes) you have configured for each album.

The app caches image keys for 1 hour in memory. If you want to clear that cache, restart the server binary and that's it.

The frontend uses [echo](https://github.com/toddmotto/echo) to lazy load images that are not in view. It also unloads images that scroll out of the view. This was done because we usually have albums with tons of images, and having them all loaded at once would hog memory.

## Final thoughts
50mm was created because of a frustration we felt. As amateur photographers, we take lots of photographs, and didn't find an easy solution to share those photos with our friends and family. 50mm is our answer to that frustration.

After having used 50mm for more than a month, we feel that for _us_, this is the solution we wanted. But there are a lot of missing features. Here's an initial list of things we plan to add:
- Ability to view the original photo. If you use Imgix, the photos displayed on the site is resized down to best fit in the web page. But sometimes people need the originals, to edit, crop, whatever. We have plans to add small overlay buttons on the photos to allow that.
- Download the entire album. If you're sharing photos from a trip, some of the people in those photos might want to keep a local copy of the entire album. Right now there's no easy way to download the entire album.

We'd love to hear feedback from other developers and amateur photographers about 50mm. If you want to use this for your own site but are not a developer (or are not that familiar with setting up software on web servers), we'd love to help you there as well.

Hit us up on hello@agileleaf.com.
