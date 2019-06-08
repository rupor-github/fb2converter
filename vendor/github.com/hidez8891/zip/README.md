update archive/zip

## zip.Writer

zip.Writer supports a format that does not have a data-descriptor.  
If you don't use data descriptor, writer needs to implement io.WriterAt.

### Writer.CopyFile

zip.Writer can write zip.Reader's File.

```go
r, _ := zip.NewReader(inputReader, inputSize)
w, _ := zip.NewWriter(outputWriter)

for _, file := range r.File {
    // if you don't want a data descriptor,
    // you need unset data descriptor flag.
    file.Flags &= ^zip.FlagDataDescriptor

    // copy zip entry
    w.CopyFile(file)
}
```

## zip.Updater

zip.Updater provides editing of zip files.

```go
u, _ := zip.NewUpdater(inputReader, inputSize)

// read file
r, _ := u.Open(readFileName)
r.Read(readFileContents)
r.Close()

// add new file
w, _ := u.Create(addFileName)
w.Write(addFileContents)
w.Close()

// update file
w, _ = u.Update(updateFileName)
w.Write(updateFileContents)
w.Close()

// rename file
u.Rename(oldFileName, newFileName)

// remove file
u.Remove(removeFileName)

// sort
u.Sort(func(s []string)[]string {
    return newStringSlice(s)
})

// save
u.SaveAs(outputWriter)
```
