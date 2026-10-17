' Paperworks office refresh. Runs as an application macro inside a
' throwaway profile; documents are opened with macro execution disabled.
Function P(n As String, v As Variant) As com.sun.star.beans.PropertyValue
  Dim x As New com.sun.star.beans.PropertyValue
  x.Name = n : x.Value = v
  P = x
End Function

Sub Fail(msg As String)
  Dim f As Integer
  f = FreeFile
  Open Environ("PW_STATUS") For Output As #f
  Print #f, "error: " & msg
  Close #f
End Sub

Sub Main
  Dim doc As Object
  On Error GoTo Failed
  Dim args(2) As New com.sun.star.beans.PropertyValue
  args(0) = P("Hidden", True)
  args(1) = P("MacroExecutionMode", com.sun.star.document.MacroExecMode.NEVER_EXECUTE)
  args(2) = P("UpdateDocMode", com.sun.star.document.UpdateDocMode.NO_UPDATE)
  doc = StarDesktop.loadComponentFromURL(Environ("PW_IN"), "_blank", 0, args())
  If IsNull(doc) Then
    Fail("Office could not open the source copy")
    GoTo Done
  End If
  If HasUnoInterfaces(doc, "com.sun.star.sheet.XCalculatable") Then doc.calculateAll()
  If HasUnoInterfaces(doc, "com.sun.star.text.XTextFieldsSupplier") Then doc.getTextFields().refresh()
  If HasUnoInterfaces(doc, "com.sun.star.text.XDocumentIndexesSupplier") Then
    Dim idx As Object, i As Integer
    idx = doc.getDocumentIndexes()
    For i = 0 To idx.getCount() - 1
      idx.getByIndex(i).update()
    Next i
    If HasUnoInterfaces(doc, "com.sun.star.text.XTextFieldsSupplier") Then doc.getTextFields().refresh()
  End If
  If Environ("PW_WHOLE_SHEETS") = "1" Then WholeSheets(doc)
  Dim opts(1) As New com.sun.star.beans.PropertyValue
  opts(0) = P("FilterName", Environ("PW_FILTER"))
  opts(1) = P("Overwrite", False)
  doc.storeToURL(Environ("PW_OUT"), opts())
  Dim f As Integer
  f = FreeFile
  Open Environ("PW_STATUS") For Output As #f
  Print #f, "ok"
  Close #f
  GoTo Done
Failed:
  Fail("line " & Erl & ": " & Error$)
Done:
  On Error Resume Next
  If Not IsNull(doc) And Not IsEmpty(doc) Then doc.close(True)
  StarDesktop.terminate()
End Sub

' Size each sheet's page to its content so headings and grids survive
' without page breaks; visibility is restored by page selection afterwards.
Sub WholeSheets(doc As Object)
  Dim sheets As Object, styles As Object, n As Integer, i As Integer, j As Integer
  sheets = doc.getSheets()
  styles = doc.getStyleFamilies().getByName("PageStyles")
  n = sheets.getCount()
  For i = 0 To n - 1
    sheets.getByIndex(i).IsVisible = True
  Next i
  Dim bo(0) As New com.sun.star.beans.PropertyValue
  bo(0) = P("SinglePageSheets", True)
  If doc.getRendererCount(doc, bo()) <> n Then Error 1000
  Dim bounds() As Variant
  ReDim bounds(n - 1)
  For i = 0 To n - 1
    bounds(i) = doc.getRenderer(i, doc, bo())
  Next i
  For i = 0 To n - 1
    Dim sheet As Object, oldStyle As Object, headings As Boolean, grid As Boolean
    sheet = sheets.getByIndex(i)
    oldStyle = styles.getByName(sheet.PageStyle)
    headings = oldStyle.PrintHeaders
    grid = oldStyle.PrintGrid
    Dim rng As Variant, sz As Variant, found As Integer
    found = 0
    For j = LBound(bounds(i)) To UBound(bounds(i))
      If bounds(i)(j).Name = "SourceRange" Then rng = bounds(i)(j).Value : found = found + 1
      If bounds(i)(j).Name = "PageSize" Then sz = bounds(i)(j).Value : found = found + 1
    Next j
    If found < 2 Then Error 1001
    Dim isBlank As Boolean
    isBlank = rng.EndColumn < rng.StartColumn Or rng.EndRow < rng.StartRow
    If isBlank Then
      rng.StartColumn = 0 : rng.EndColumn = 0 : rng.StartRow = 0 : rng.EndRow = 0
    End If
    rng.Sheet = i
    If rng.StartColumn < 0 Then rng.StartColumn = 0
    If rng.StartRow < 0 Then rng.StartRow = 0
    If rng.EndColumn < rng.StartColumn Then rng.EndColumn = rng.StartColumn
    If rng.EndRow < rng.StartRow Then rng.EndRow = rng.StartRow
    Dim style As Object, nm As String, w As Long, h As Long
    style = doc.createInstance("com.sun.star.style.PageStyle")
    nm = "DocumentPreview_" & i & "_" & Int(Rnd() * 1000000000)
    styles.insertByName(nm, style)
    style.LeftMargin = 0 : style.RightMargin = 0 : style.TopMargin = 0 : style.BottomMargin = 0
    style.HeaderIsOn = False : style.FooterIsOn = False
    style.PrintHeaders = headings : style.PrintGrid = grid
    w = sz.Width + 100 : h = sz.Height + 100
    If headings Then w = w + 1100 : h = h + 600
    If w < 1000 Then w = 1000
    If h < 1000 Then h = 1000
    style.Width = w : style.Height = h
    style.ScaleToPagesX = 1 : style.ScaleToPagesY = 1
    sheet.PageStyle = nm
    sheet.setPrintTitleRows(False)
    sheet.setPrintTitleColumns(False)
    Dim areas(0) As New com.sun.star.table.CellRangeAddress
    areas(0) = rng
    sheet.setPrintAreas(areas())
    If isBlank Then
      Dim shape As Object, pt As New com.sun.star.awt.Point, s2 As New com.sun.star.awt.Size
      shape = doc.createInstance("com.sun.star.drawing.RectangleShape")
      s2.Width = 100 : s2.Height = 100
      shape.setPosition(pt) : shape.setSize(s2)
      sheet.getDrawPage().add(shape)
      shape.FillStyle = com.sun.star.drawing.FillStyle.NONE
      shape.LineStyle = com.sun.star.drawing.LineStyle.NONE
    End If
  Next i
End Sub
