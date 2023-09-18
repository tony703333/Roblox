local Library = loadstring(game:HttpGet("https://raw.githubusercontent.com/xHeptc/Kavo-UI-Library/main/source.lua"))()
local Window = Library.CreateLib("TeddyBear Hub--外掛(V1.1)", "GrapeTheme")

local Other = Window:NewTab("其他外掛")
local OtherSection = Other:NewSection("其他外掛")

OtherSection:NewButton("Blox Fruits", "其他腳本", function()
    getgenv().WaterMark = true
    loadstring(game:HttpGet("https://raw.githubusercontent.com/CHEATERFUN/Blox-Fruits-Pastebin-Script/main/script"))()
end)
