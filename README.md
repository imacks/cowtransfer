cowtransfer
===========
This is a client for https://cowtransfer.cn

Most code is from https://github.com/Mikubill/cowtransfer-uploader. I have removed dependency on 
`github.com/cheggaaa/pb` in favor of various progress event hooks. The intention is to decouple 
the CLI from the library API. 


Prepare your file
-----------------
Download the file you want to your remote machine:

```bash
curl -Lo myfile.dat "http://www.example.com/myfile.dat"
```

Cowtransfer offers a password protection feature, but why trust 'em when you can encrypt locally? Of course this step is optional.

```bash
openssl enc -aes-256-cbc -in ./myfile.dat -out myfilec.dat
```

Sometimes cowtransfer have problems with large files. If that happens, chop things up to smaller bits! The command below will 
create myfilec.aa, myfilec.ab, ..., each file being 512mb.

```bash
split --verbose -b 512M myfilec.dat myfilec.dat.
ls -lh myfilec.dat.*
```

Upload to Cowtransfer
---------------------
Install the latest version of cowtransfer CLI client. Example for Ubuntu shown below:

```bash
latest_ver=$(curl -fsSL "https://api.github.com/repos/imacks/cowtransfer/releases/latest" | grep tag_name | cut -d'"' -f4)
curl -Lo cowtransfer "https://github.com/imacks/cowtransfer/releases/download/${latest_ver}/cowtransfer_ubuntu"
chmod +x cowtransfer
./cowtransfer -h
```

Time to upload:

```bash
files=$(ls -1 myfilec.dat.*)
./cowtransfer -V $files
```

Lots of progress messages follows, but look out for the final download link. Here's an example:

```
link: https://cowtransfer.com/s/abab0000123456
```

Now you can use your local computer to visit the URL. You may simply choose to download what you want from the browser, but if there are 
a lot of files, read on to automate the download process too.


Download from Cowtransfer
-------------------------
You need to install cowtransfer CLI client on your local computer first. If your local PC is Linux too, installation steps are identical as for 
remote machine. I will show an example for Windows below:

```powershell
$latestver = wget -UseBasicParsing "https://api.github.com/repos/imacks/cowtransfer/releases/latest" | select -expand Content | ConvertFrom-Json | select -expand tag_name
wget -OutFile cowtransfer.exe "https://github.com/imacks/cowtransfer/releases/download/${latestver}/cowtransfer.exe"
.\cowtransfer -h
```

You need the download link for the next step:

```powershell
.\cowtransfer https://cowtransfer.com/s/abab0000123456
```

This will get the actual direct download URLs for all the files. Download them using your favorite download tool.

On Windows, use the awesome 7-zip to open any of the downloaded files. 7-zip can handle decryption and split files.

A bit more work is necessary if using Linux:

To join split files:

```bash
cat myfilec.dat.* > myfilec.dat
```

To decrypt your file:

```bash
openssl enc -aes-256-cbc -d -in myfilec.dat > myfile.dat
```


Cleaning up
-----------
You may want to clear up your remote machine to save some disk space:

```bash
rm myfile.dat myfilec.dat myfilec.dat.*
```
