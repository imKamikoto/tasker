# Иконка Tasker

Геометрия: squircle-радиус 22.37% от стороны · глиф 42% высоты · каретка 12.5% × 30% с отступом 2.2% от «t» · точечная вуаль сеткой 4% с радиусом ≤ 0.3 шага, альфа падает от точки 30/28% · на размерах ≤ 32px вуаль не рисуется.

Цвета: тёмная плитка #212121 → #0F0F0F, глиф #EBEBEB, каретка #8AA6BF (акцент Сталь). Светлая плитка #FCFCFC → #E5E5E5, глиф #1F1F1F, каретка #365E86.

Файлы: tasker-1024/512/256/128/64/32/16.png (тёмная), tasker-light-1024/256.png (для меню-бара и светлой темы).

Сборка .icns:

    mkdir Tasker.iconset
    cp tasker-16.png   Tasker.iconset/icon_16x16.png
    cp tasker-32.png   Tasker.iconset/icon_16x16@2x.png
    cp tasker-32.png   Tasker.iconset/icon_32x32.png
    cp tasker-64.png   Tasker.iconset/icon_32x32@2x.png
    cp tasker-128.png  Tasker.iconset/icon_128x128.png
    cp tasker-256.png  Tasker.iconset/icon_128x128@2x.png
    cp tasker-256.png  Tasker.iconset/icon_256x256.png
    cp tasker-512.png  Tasker.iconset/icon_256x256@2x.png
    cp tasker-512.png  Tasker.iconset/icon_512x512.png
    cp tasker-1024.png Tasker.iconset/icon_512x512@2x.png
    iconutil -c icns Tasker.iconset
