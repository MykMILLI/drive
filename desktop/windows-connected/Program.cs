using System;
using System.Collections.Generic;
using System.Drawing;
using System.IO;
using System.Net.Http;
using System.Security.Cryptography;
using System.Text;
using System.Threading;
using System.Threading.Tasks;
using System.Web.Script.Serialization;
using System.Windows.Forms;
namespace Millitop.DriveConnected {
 public class Bootstrap { public string server {get;set;} }
 public class Saved { public string Server{get;set;} public string LocalFolder{get;set;} public string ProtectedToken{get;set;} public Dictionary<string,string> Uploaded{get;set;} }
 public class ApiError:Exception {public int Status;public ApiError(int s):base("Server returned HTTP "+s){Status=s;}}
 public class Payload { public byte[] Bytes;public string Hash; }
 static class Program {
  [STAThread]static void Main(string[] args){bool first;using(var mutex=new Mutex(true,"Local\\MILLITOP-Drive-Connected-"+Environment.UserName,out first)){if(!first){MessageBox.Show("Drive Desktop is already running. Exit it from the tray before launching this revision.");return;}Application.EnableVisualStyles();Application.SetCompatibleTextRenderingDefault(false);try{Application.Run(new Agent(Array.IndexOf(args,"--tray")>=0));}catch(Exception e){MessageBox.Show(e.Message,"Drive Desktop");}}}
 }
 public sealed class Agent:Form {
  readonly JavaScriptSerializer json=new JavaScriptSerializer();readonly HttpClient client;readonly NotifyIcon tray=new NotifyIcon();readonly System.Windows.Forms.Timer timer=new System.Windows.Forms.Timer();readonly FileSystemWatcher watcher=new FileSystemWatcher();
  readonly Label status=new Label(),remoteLabel=new Label();readonly TextBox folder=new TextBox();readonly Button connect=new Button();readonly CheckBox startup=new CheckBox();readonly ToolStripMenuItem pause=new ToolStripMenuItem("Pause transfers");
  readonly string server,settingsDir;readonly bool hidden;readonly Dictionary<string,string> uploaded=new Dictionary<string,string>(StringComparer.OrdinalIgnoreCase);
  string token="",requestId="",verifier="";DateTime deadline,nextPoll=DateTime.MinValue,nextCheck=DateTime.MinValue;bool busy,paused,exiting;
  const int MaxBytes=32*1024*1024;static readonly byte[] entropy=Encoding.UTF8.GetBytes("MILLITOP Drive connected agent v0.2");
  string SettingsFile{get{return Path.Combine(settingsDir,"settings.json");}}string Shortcut{get{return Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.Startup),"MILLITOP Drive Connected.lnk");}}
  public Agent(bool startHidden){
   hidden=startHidden;string bootstrapPath=Path.Combine(AppDomain.CurrentDomain.BaseDirectory,"instance.json");if(!File.Exists(bootstrapPath))throw new Exception("Extract the complete package downloaded from your Drive website, including instance.json.");
   var bootstrap=json.Deserialize<Bootstrap>(File.ReadAllText(bootstrapPath));Uri uri;if(bootstrap==null||!Uri.TryCreate(bootstrap.server,UriKind.Absolute,out uri)||(uri.Scheme!="https"&&uri.Scheme!="http")||uri.UserInfo!=""||uri.Query!=""||uri.Fragment!="")throw new Exception("Invalid instance configuration.");server=uri.AbsoluteUri.TrimEnd('/');
   byte[] urlHash;using(var sha=SHA256.Create())urlHash=sha.ComputeHash(Encoding.UTF8.GetBytes(server));settingsDir=Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData),"MILLITOP","DriveConnected",BitConverter.ToString(urlHash,0,8).Replace("-",""));
   Text="MILLITOP Drive Desktop - Two-way New-Item Preview";ClientSize=new Size(700,470);Font=new Font("Segoe UI",10);FormBorderStyle=FormBorderStyle.FixedDialog;MaximizeBox=false;
   var instance=new Label{Text="Instance: "+server,Left=22,Top=20,Width=650,Height=45};Controls.Add(instance);
   connect.SetBounds(22,75,260,36);connect.Click+=async delegate{if(requestId!=""){requestId="";verifier="";connect.Text="Connect / approve uploads";SetStatus("Connection canceled.",SystemIcons.Warning);}else await StartConnection();};Controls.Add(connect);
   var label=new Label{Text="Local folder: new files and folders transfer both ways; existing files are not replaced",Left=22,Top=135,Width=655,Height=40};Controls.Add(label);folder.SetBounds(22,182,515,29);folder.ReadOnly=true;Controls.Add(folder);
   var browse=new Button{Text="Browse...",Left=550,Top=180,Width=115,Height=32};browse.Click+=delegate{if(busy){MessageBox.Show(this,"Wait for the current upload pass to finish before changing folders.");return;}using(var dialog=new FolderBrowserDialog()){dialog.ShowNewFolderButton=false;if(Directory.Exists(folder.Text))dialog.SelectedPath=folder.Text;if(dialog.ShowDialog(this)==DialogResult.OK){folder.Text=dialog.SelectedPath;SaveSettings();Watch();nextCheck=DateTime.MinValue;}}};Controls.Add(browse);
   startup.SetBounds(22,232,650,28);startup.Text="Start in the tray when I sign in";startup.Checked=File.Exists(Shortcut);startup.CheckedChanged+=delegate{try{SetStartup();}catch(Exception e){MessageBox.Show(this,e.Message);}};Controls.Add(startup);
   remoteLabel.SetBounds(22,280,650,30);remoteLabel.Text="Remote folder: checking";Controls.Add(remoteLabel);status.SetBounds(22,325,650,125);Controls.Add(status);
   client=new HttpClient(new HttpClientHandler{AllowAutoRedirect=false,UseCookies=false}){Timeout=TimeSpan.FromSeconds(30),MaxResponseContentBufferSize=MaxBytes};
   var menu=new ContextMenuStrip();menu.Items.Add("Open settings",null,delegate{Show();WindowState=FormWindowState.Normal;Activate();});menu.Items.Add("Check / upload now",null,async delegate{nextCheck=DateTime.MinValue;await Tick();});pause.Click+=delegate{paused=!paused;pause.Text=paused?"Resume transfers":"Pause transfers";SetStatus(paused?"Paused. An in-flight upload may finish.":"Transfers resumed.",SystemIcons.Information);nextCheck=DateTime.MinValue;};menu.Items.Add(pause);menu.Items.Add("Open Drive",null,delegate{try{System.Diagnostics.Process.Start(server+"/files/");}catch{}});menu.Items.Add(new ToolStripSeparator());menu.Items.Add("Exit",null,delegate{exiting=true;Close();});tray.Icon=SystemIcons.Application;tray.Text="Drive - new-file upload preview";tray.ContextMenuStrip=menu;tray.Visible=true;tray.DoubleClick+=delegate{Show();Activate();};
   FormClosing+=delegate(object sender,FormClosingEventArgs e){if(!exiting&&e.CloseReason==CloseReason.UserClosing){e.Cancel=true;Hide();}};
   watcher.SynchronizingObject=this;watcher.IncludeSubdirectories=true;watcher.NotifyFilter=NotifyFilters.FileName|NotifyFilters.DirectoryName|NotifyFilters.LastWrite|NotifyFilters.Size;watcher.Created+=delegate{nextCheck=DateTime.UtcNow.AddSeconds(2);};watcher.Changed+=delegate{nextCheck=DateTime.UtcNow.AddSeconds(2);};watcher.Renamed+=delegate{nextCheck=DateTime.UtcNow.AddSeconds(2);};watcher.Error+=delegate{nextCheck=DateTime.MinValue;};
   LoadSettings();Watch();connect.Text="Connect / approve uploads";SetStatus("Two-way new-item preview. Existing-file updates and deletions are not supported.",SystemIcons.Information);timer.Interval=1000;timer.Tick+=async delegate{await Tick();};Shown+=async delegate{if(hidden&&token!="")Hide();timer.Start();await Tick();};
  }
  void Watch(){watcher.EnableRaisingEvents=false;if(Directory.Exists(folder.Text)){watcher.Path=folder.Text;watcher.EnableRaisingEvents=true;}}
  void SetStatus(string text,Icon icon){if(IsDisposed)return;status.Text=text;string tip="Drive - "+text;tray.Text=tip.Substring(0,Math.Min(63,tip.Length));tray.Icon=icon;}
  static string B64(byte[] value){return Convert.ToBase64String(value).TrimEnd('=').Replace('+','-').Replace('/','_');}
  async Task<Dictionary<string,object>> Request(string path,object body,string credential){using(var req=new HttpRequestMessage(body==null?HttpMethod.Get:HttpMethod.Post,server+path)){if(body!=null)req.Content=new StringContent(json.Serialize(body),Encoding.UTF8,"application/json");if(credential!="")req.Headers.Authorization=new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer",credential);using(var res=await client.SendAsync(req)){if(!res.IsSuccessStatusCode)throw new ApiError((int)res.StatusCode);json.MaxJsonLength=4*1024*1024;var result=json.DeserializeObject(await res.Content.ReadAsStringAsync()) as Dictionary<string,object>;if(result==null)throw new Exception("Unexpected server response.");return result;}}}
  async Task StartConnection(){if(busy)return;if(server.StartsWith("http://",StringComparison.OrdinalIgnoreCase)&&MessageBox.Show(this,"HTTP exposes your credential and uploaded content to network observers. Use only for a trusted-LAN development test. Continue?","Unencrypted HTTP",MessageBoxButtons.YesNo,MessageBoxIcon.Warning)!=DialogResult.Yes)return;busy=true;connect.Enabled=false;try{var random=new byte[32];using(var rng=RandomNumberGenerator.Create())rng.GetBytes(random);verifier=B64(random);string challenge;using(var sha=SHA256.Create())challenge=B64(sha.ComputeHash(Encoding.ASCII.GetBytes(verifier)));string machine=Environment.MachineName;if(machine.Length>100)machine=machine.Substring(0,100);var result=await Request("/api/sync/pair/start",new{name=machine,challenge=challenge},"");requestId=Convert.ToString(result["requestId"]);if(!System.Text.RegularExpressions.Regex.IsMatch(requestId,"^[a-f0-9]{32}$"))throw new Exception("Invalid request ID.");deadline=DateTime.UtcNow.AddMinutes(5);nextPoll=DateTime.UtcNow.AddSeconds(3);System.Diagnostics.Process.Start(server+"/settings/connect-desktop?request="+requestId);connect.Text="Cancel connection request";SetStatus("Approve in your browser and explicitly allow two-way creation of files and folders.",SystemIcons.Information);}catch(Exception e){requestId="";verifier="";SetStatus(Explain(e),SystemIcons.Warning);}finally{busy=false;connect.Enabled=true;}}
  async Task Tick(){if(busy||IsDisposed)return;if(requestId!=""){if(DateTime.UtcNow>deadline){requestId="";verifier="";connect.Text="Connect / approve uploads";SetStatus("Approval expired. Reconnect.",SystemIcons.Warning);return;}if(DateTime.UtcNow<nextPoll)return;busy=true;nextPoll=DateTime.UtcNow.AddSeconds(3);try{var response=await Request("/api/sync/pair/claim",new{id=requestId,verifier=verifier},"");if(Convert.ToString(response["status"])=="approved"){token=Convert.ToString(response["token"]);if(!token.StartsWith("ds1."))throw new Exception("Invalid credential.");requestId="";verifier="";connect.Text="Connect / approve uploads";SaveSettings();nextCheck=DateTime.MinValue;SetStatus("Connected. Checking upload authorization.",SystemIcons.Information);}}catch(ApiError e){if(e.Status!=429){requestId="";verifier="";connect.Text="Connect / approve uploads";SetStatus(Explain(e),SystemIcons.Warning);}}catch(Exception e){SetStatus(Explain(e),SystemIcons.Warning);}finally{busy=false;}return;}if(paused||token==""||DateTime.UtcNow<nextCheck)return;busy=true;nextCheck=DateTime.UtcNow.AddSeconds(10);try{await UploadPass();}catch(Exception e){SetStatus(Explain(e),SystemIcons.Warning);}finally{busy=false;}}
  static Payload Snapshot(string path){
   if((File.GetAttributes(path)&FileAttributes.ReparsePoint)!=0)throw new Exception("Reparse-point files are skipped.");
   using(var file=new FileStream(path,FileMode.Open,FileAccess.Read,FileShare.Read)){if(file.Length>MaxBytes)throw new Exception("File exceeds 32 MiB preview limit.");using(var memory=new MemoryStream()){var buffer=new byte[65536];int count;while((count=file.Read(buffer,0,buffer.Length))>0){if(memory.Length+count>MaxBytes)throw new Exception("File exceeds limit.");memory.Write(buffer,0,count);}var bytes=memory.ToArray();using(var sha=SHA256.Create())return new Payload{Bytes=bytes,Hash=BitConverter.ToString(sha.ComputeHash(bytes)).Replace("-","").ToLowerInvariant()};}}
  }
  class TreeEntry {public string Path;public bool IsDir;}
  class LocalTree {public List<string> Files=new List<string>();public List<string> Dirs=new List<string>();public int Skipped;}
  static void ValidateRelative(string relative){
   if(String.IsNullOrEmpty(relative)||relative.Length>4096||relative.StartsWith("/")||relative.IndexOf('\\')>=0)throw new Exception("Unsupported server path.");
   foreach(string part in relative.Split('/')){
    if(part==""||part=="."||part==".."||part.EndsWith(".")||part.EndsWith(" ")||part.IndexOfAny(Path.GetInvalidFileNameChars())>=0)throw new Exception("Unsupported Windows name: "+relative);
    string stem=part.Split('.')[0].ToUpperInvariant();
    if(stem=="CON"||stem=="PRN"||stem=="AUX"||stem=="NUL"||System.Text.RegularExpressions.Regex.IsMatch(stem,"^(COM|LPT)[1-9]$"))throw new Exception("Reserved Windows name: "+relative);
   }
  }
  static string LocalPath(string root,string relative){
   ValidateRelative(relative);string prefix=Path.GetFullPath(root).TrimEnd(Path.DirectorySeparatorChar)+Path.DirectorySeparatorChar;
   string result=Path.GetFullPath(Path.Combine(prefix,relative.Replace('/',Path.DirectorySeparatorChar)));
   if(!result.StartsWith(prefix,StringComparison.OrdinalIgnoreCase))throw new Exception("Path escapes the selected local folder.");
   string current=Path.GetFullPath(root);
   if((File.GetAttributes(current)&FileAttributes.ReparsePoint)!=0)throw new Exception("Reparse-point root is not supported.");
   foreach(string part in relative.Split('/')){current=Path.Combine(current,part);if((File.Exists(current)||Directory.Exists(current))&&(File.GetAttributes(current)&FileAttributes.ReparsePoint)!=0)throw new Exception("Reparse-point path skipped: "+relative);}
   return result;
  }
  static LocalTree ScanTree(string root){
   if((File.GetAttributes(root)&FileAttributes.ReparsePoint)!=0)throw new Exception("Reparse-point root is not supported.");
   var result=new LocalTree();var stack=new Stack<string>();stack.Push(root);int visited=0;string prefix=root.TrimEnd(Path.DirectorySeparatorChar)+Path.DirectorySeparatorChar;
   while(stack.Count>0){string dir=stack.Pop();foreach(string path in Directory.EnumerateFileSystemEntries(dir)){
    if(++visited>5000)throw new Exception("Local tree exceeds the 5000-entry preview limit. Select a smaller test folder.");
    string name=Path.GetFileName(path);if(name.StartsWith(".drive-sync-",StringComparison.Ordinal)||name.StartsWith("~$",StringComparison.Ordinal)){result.Skipped++;continue;}
    var attributes=File.GetAttributes(path);if((attributes&FileAttributes.ReparsePoint)!=0){result.Skipped++;continue;}
    string relative=path.Substring(prefix.Length).Replace(Path.DirectorySeparatorChar,'/');ValidateRelative(relative);
    if(relative.Split('/').Length>64)throw new Exception("Folder nesting exceeds preview limit.");
    if((attributes&FileAttributes.Directory)!=0){result.Dirs.Add(relative);stack.Push(path);}else result.Files.Add(relative);
   }}
   result.Dirs.Sort(delegate(string a,string b){int depth=a.Split('/').Length.CompareTo(b.Split('/').Length);return depth!=0?depth:StringComparer.OrdinalIgnoreCase.Compare(a,b);});result.Files.Sort(StringComparer.OrdinalIgnoreCase);return result;
  }
  async Task<string> DownloadNewFile(string relative,string localRoot){
   string destination=LocalPath(localRoot,relative);if(File.Exists(destination)||Directory.Exists(destination))return null;
   string parent=Path.GetDirectoryName(destination);Directory.CreateDirectory(parent);LocalPath(localRoot,relative);
   byte[] bytes;string expected=null;
   using(var req=new HttpRequestMessage(HttpMethod.Get,server+"/api/sync/device/download?name="+Uri.EscapeDataString(relative))){req.Headers.Authorization=new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer",token);using(var response=await client.SendAsync(req)){
    if(!response.IsSuccessStatusCode)throw new ApiError((int)response.StatusCode);IEnumerable<string> values;if(response.Headers.TryGetValues("X-Content-SHA256",out values))foreach(string value in values){expected=value;break;}
    bytes=await response.Content.ReadAsByteArrayAsync();if(bytes.Length>MaxBytes||String.IsNullOrEmpty(expected))throw new Exception("Unsupported or oversized download.");
   }}
   string digest;using(var sha=SHA256.Create())digest=BitConverter.ToString(sha.ComputeHash(bytes)).Replace("-","").ToLowerInvariant();if(!String.Equals(digest,expected,StringComparison.OrdinalIgnoreCase))throw new Exception("Download digest mismatch.");
   string temp=Path.Combine(parent,".drive-sync-download-"+Guid.NewGuid().ToString("N"));
   try{using(var file=new FileStream(temp,FileMode.CreateNew,FileAccess.Write,FileShare.None)){await file.WriteAsync(bytes,0,bytes.Length);file.Flush(true);}LocalPath(localRoot,relative);File.Move(temp,destination);}finally{if(File.Exists(temp))File.Delete(temp);}
   return digest;
  }
  async Task UploadPass(){
   var inventory=await Request("/api/sync/device/inventory",null,token);if(!inventory.ContainsKey("complete")||!Object.Equals(inventory["complete"],true)||!(inventory["entries"] is object[]))throw new Exception("Incomplete inventory. No transfers attempted.");
   string root=Convert.ToString(inventory["root"]);remoteLabel.Text="Remote folder: "+root;
   var remote=new Dictionary<string,TreeEntry>(StringComparer.OrdinalIgnoreCase);
   foreach(var obj in (object[])inventory["entries"]){var entry=obj as Dictionary<string,object>;if(entry==null)throw new Exception("Invalid inventory.");string relative=Convert.ToString(entry["path"]);ValidateRelative(relative);if(relative.Split('/').Length>64)throw new Exception("Server tree exceeds nesting limit.");if(remote.ContainsKey(relative))throw new Exception("Server names collide on Windows: "+relative);remote.Add(relative,new TreeEntry{Path=relative,IsDir=Object.Equals(entry["isDir"],true)});}
   if(!Directory.Exists(folder.Text)){SetStatus("Choose an existing local test folder. Remote entries: "+remote.Count,SystemIcons.Warning);return;}
   string localRoot=Path.GetFullPath(folder.Text);LocalTree local=await Task.Run(()=>ScanTree(localRoot));
   var capability=await Request("/api/sync/device/capabilities",null,token);
   if(!capability.ContainsKey("newFileUpload")||!Object.Equals(capability["newFileUpload"],true)||!capability.ContainsKey("recursive")||!Object.Equals(capability["recursive"],true)||!capability.ContainsKey("download")||!Object.Equals(capability["download"],true)){
    SetStatus("Local files: "+local.Files.Count+"; folders: "+local.Dirs.Count+". Remote entries: "+remote.Count+". Reconnect and explicitly approve two-way file/folder transfers. Account also needs create and download permission.",SystemIcons.Warning);return;
   }
   int sent=0,received=0,dirsUp=0,dirsDown=0,conflicts=0,failed=0,pending=0,actions=0;string lastError="";
   foreach(string relative in local.Dirs){if(paused||IsDisposed)break;TreeEntry existing;if(remote.TryGetValue(relative,out existing)){if(!existing.IsDir)conflicts++;continue;}if(actions>=20){pending++;continue;}try{SetStatus("Creating remote folder "+relative,SystemIcons.Information);await Request("/api/sync/device/mkdir?name="+Uri.EscapeDataString(relative),new{},token);remote[relative]=new TreeEntry{Path=relative,IsDir=true};dirsUp++;actions++;}catch(Exception e){failed++;lastError=relative+": "+Explain(e);}}
   var serverDirs=new List<string>();var serverFiles=new List<string>();foreach(var entry in remote.Values){if(entry.IsDir)serverDirs.Add(entry.Path);else serverFiles.Add(entry.Path);}serverDirs.Sort(delegate(string a,string b){return a.Length.CompareTo(b.Length);});serverFiles.Sort(StringComparer.OrdinalIgnoreCase);
   foreach(string relative in serverDirs){if(paused||IsDisposed)break;try{string path=LocalPath(localRoot,relative);if(File.Exists(path)){conflicts++;continue;}if(!Directory.Exists(path)){Directory.CreateDirectory(path);dirsDown++;}}catch(Exception e){failed++;lastError=relative+": "+e.Message;}}
   foreach(string relative in serverFiles){if(paused||IsDisposed)break;try{string path=LocalPath(localRoot,relative);if(Directory.Exists(path)){conflicts++;continue;}if(File.Exists(path))continue;if(actions>=20){pending++;continue;}SetStatus("Downloading "+relative,SystemIcons.Information);string digest=await DownloadNewFile(relative,localRoot);if(digest!=null){received++;actions++;uploaded[localRoot+"|"+root+"|"+relative]=digest;SaveSettings();}}catch(ApiError e){if(e.Status==401||e.Status==403)throw;failed++;lastError=relative+": "+Explain(e);}catch(Exception e){failed++;lastError=relative+": "+e.Message;}}
   foreach(string relative in local.Files){if(paused||IsDisposed)break;try{string path=LocalPath(localRoot,relative);Payload payload=await Task.Run(()=>Snapshot(path));string key=localRoot+"|"+root+"|"+relative;TreeEntry existing;
    if(remote.TryGetValue(relative,out existing)){string previous;if(existing.IsDir||!uploaded.TryGetValue(key,out previous)||previous!=payload.Hash)conflicts++;continue;}
    if(actions>=20){pending++;continue;}SetStatus("Uploading "+relative,SystemIcons.Information);
    using(var req=new HttpRequestMessage(HttpMethod.Post,server+"/api/sync/device/upload?name="+Uri.EscapeDataString(relative))){req.Headers.Authorization=new System.Net.Http.Headers.AuthenticationHeaderValue("Bearer",token);req.Headers.TryAddWithoutValidation("If-None-Match","*");req.Headers.Add("X-Content-SHA256",payload.Hash);req.Content=new ByteArrayContent(payload.Bytes);req.Content.Headers.ContentType=new System.Net.Http.Headers.MediaTypeHeaderValue("application/octet-stream");req.Content.Headers.ContentLength=payload.Bytes.LongLength;using(var response=await client.SendAsync(req)){if((int)response.StatusCode==412){conflicts++;continue;}if((int)response.StatusCode!=201)throw new ApiError((int)response.StatusCode);}}
    sent++;actions++;remote[relative]=new TreeEntry{Path=relative,IsDir=false};uploaded[key]=payload.Hash;SaveSettings();
   }catch(ApiError e){if(e.Status==401||e.Status==403)throw;failed++;lastError=relative+": "+Explain(e);}catch(Exception e){failed++;lastError=relative+": "+e.Message;}}
   SetStatus("Local scan: "+local.Files.Count+" files, "+local.Dirs.Count+" folders. Uploaded: "+sent+"; downloaded: "+received+"; folders created remote/local: "+dirsUp+"/"+dirsDown+". Existing/unverified conflicts: "+conflicts+"; errors: "+failed+"; pending: "+pending+"; skipped: "+local.Skipped+".\r\n"+(lastError!=""?lastError:"New items in both directions only. Existing-file updates and deletions remain disabled. Counts reflect this pass's starting scan."),failed>0||conflicts>0?SystemIcons.Warning:SystemIcons.Information);
   if(pending>0)nextCheck=DateTime.UtcNow.AddSeconds(2);
  }
  string Explain(Exception e){var api=e as ApiError;if(api!=null){if(api.Status==401)return "Credential expired, invalid or revoked; reconnect.";if(api.Status==403)return "Access denied; check permissions and upload approval.";if(api.Status==404)return "API or selected folder not found; check deployment.";if(api.Status==413)return "File exceeds server upload limit.";return api.Message;}if(e is HttpRequestException)return "Connection failed; check network and HTTPS certificate.";if(e is TaskCanceledException)return "Server request timed out.";return e.Message;}
  void LoadSettings(){if(!File.Exists(SettingsFile))return;try{var saved=json.Deserialize<Saved>(File.ReadAllText(SettingsFile));if(saved.Server!=server)return;folder.Text=saved.LocalFolder??"";if(saved.Uploaded!=null)foreach(var pair in saved.Uploaded)uploaded[pair.Key]=pair.Value;if(String.IsNullOrEmpty(saved.ProtectedToken))return;var bytes=ProtectedData.Unprotect(Convert.FromBase64String(saved.ProtectedToken),entropy,DataProtectionScope.CurrentUser);try{token=Encoding.UTF8.GetString(bytes);}finally{Array.Clear(bytes,0,bytes.Length);}}catch{token="";}}
  void SaveSettings(){try{Directory.CreateDirectory(settingsDir);string encrypted="";if(token!=""){var bytes=Encoding.UTF8.GetBytes(token);try{encrypted=Convert.ToBase64String(ProtectedData.Protect(bytes,entropy,DataProtectionScope.CurrentUser));}finally{Array.Clear(bytes,0,bytes.Length);}}var saved=new Saved{Server=server,LocalFolder=folder.Text,ProtectedToken=encrypted,Uploaded=uploaded};string tmp=SettingsFile+".tmp";File.WriteAllText(tmp,json.Serialize(saved),new UTF8Encoding(false));if(File.Exists(SettingsFile))File.Replace(tmp,SettingsFile,null);else File.Move(tmp,SettingsFile);}catch{SetStatus("Could not save local state. Files are never overwritten as a fallback.",SystemIcons.Warning);}}
  void SetStartup(){if(!startup.Checked){if(File.Exists(Shortcut))File.Delete(Shortcut);return;}dynamic shell=Activator.CreateInstance(Type.GetTypeFromProgID("WScript.Shell"));dynamic link=shell.CreateShortcut(Shortcut);try{link.TargetPath=Application.ExecutablePath;link.Arguments="--tray";link.WorkingDirectory=AppDomain.CurrentDomain.BaseDirectory;link.Save();}finally{System.Runtime.InteropServices.Marshal.FinalReleaseComObject(link);System.Runtime.InteropServices.Marshal.FinalReleaseComObject(shell);}}
  protected override void Dispose(bool disposing){if(disposing){timer.Stop();timer.Dispose();watcher.Dispose();tray.Visible=false;tray.Dispose();client.Dispose();token="";verifier="";}base.Dispose(disposing);}
 }
}
