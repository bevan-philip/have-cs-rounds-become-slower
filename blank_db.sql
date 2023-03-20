CREATE TABLE [game] (
id INTEGER NOT NULL PRIMARY KEY,
time DATETIME NOT NULL,
team_a TEXT NOT NULL,
team_b TEXT NOT NULL,
team_a_players JSON DEFAULT('[]'),
team_b_players JSON DEFAULT('[]'), map string, tickrate INTEGER);
CREATE TABLE [round] (
round_id INTEGER NOT NULL PRIMARY KEY,
game_id INTEGER,
duration INTEGER,
losingTeamName TEXT,
losingSide INTEGER,
startTick INTEGER,
endTick INTEGER,
endOfficialTick INTEGER,
survivingPlayers JSON DEFAULT('[]'),
losingTeamLeftoverMoney INTEGER,
equipmentSavedValue INTEGER,
killTicks JSON DEFAULT('[]'),
smokeTicks JSON DEFAULT('[]'),
molotovTicks JSON DEFAULT('[]'),
heTicks JSON DEFAULT('[]'),
longestKillWait INTEGER,
lastKillToEnd INTEGER, round_no int, heDamage int,
FOREIGN KEY (game_id) REFERENCES game(id));
CREATE TABLE [players] (
SteamID64 INTEGER NOT NULL PRIMARY KEY,
name TEXT, apps INTEGER);