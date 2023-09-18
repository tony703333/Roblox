local Library = loadstring(game:HttpGet("https://raw.githubusercontent.com/xHeptc/Kavo-UI-Library/main/source.lua"))()
--LOCAL PLAYER
 local Player = Window:NewTab("撣貊鍂��蠘��")
 local PlayerSection = Player:NewSection("撣貊鍂��蠘��")

 PlayerSection:NewSlider("�笔漲", "隤踵㟲�笔漲", 500, 16, function(s)
     game.Players.LocalPlayer.Character.Humanoid.WalkSpeed = s
 end)

 PlayerSection:NewSlider("頝喟���偦��", "隤踵㟲頝喟���偦��", 350, 50, function(s)
     game.Players.LocalPlayer.Character.Humanoid.JumpPower = s
 end)

 PlayerSection:NewButton("��滨蔭 �笔漲/頝�", "�Ｗ儔���𧋦���笔漲/頝喟���偦��", function()
     game.Players.LocalPlayer.Character.Humanoid.JumpPower = 50
     game.Players.LocalPlayer.Character.Humanoid.WalkSpeed = 16
 end)

PlayerSection :NewButton("憌�", "韏琿�𥕦��", function()
        loadstring(game:HttpGet("https://raw.githubusercontent.com/CHEATERFUN/Blox-Fruits-Pastebin-Script/main/script"))()

        Fly(true)
    end)
