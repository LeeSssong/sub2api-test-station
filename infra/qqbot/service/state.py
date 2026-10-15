import json
import sqlite3
import time

class State:
    def __init__(self,path):
        self.path=str(path)
        with self.connect() as db:
            db.execute('CREATE TABLE IF NOT EXISTS events (id TEXT PRIMARY KEY, created REAL NOT NULL)')
            db.execute('CREATE TABLE IF NOT EXISTS history (session TEXT PRIMARY KEY, content TEXT NOT NULL)')
    def connect(self):
        return sqlite3.connect(self.path,timeout=10)
    def claim(self,key):
        with self.connect() as db:
            db.execute('DELETE FROM events WHERE created < ?', (time.time()-7*86400,))
            return db.execute('INSERT OR IGNORE INTO events VALUES (?,?)',(key,time.time())).rowcount==1
    def history(self,session):
        with self.connect() as db:
            r=db.execute('SELECT content FROM history WHERE session=?',(session,)).fetchone()
            return json.loads(r[0]) if r else []
    def save_history(self,session,messages):
        with self.connect() as db:
            db.execute('INSERT OR REPLACE INTO history VALUES (?,?)',(session,json.dumps(messages,ensure_ascii=False)))

