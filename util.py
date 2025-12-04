import sqlite3
from sqlite3 import Connection

def get_db_connection() -> Connection:
    conn = sqlite3.connect('portfolio.db')
    conn.row_factory = sqlite3.Row
    return conn

def update_user(user_id: str, username: str = None, email: str = None, password_hash: str = None):
    conn = get_db_connection()
    cursor = conn.cursor()
    
    fields = []
    values = []
    
    if username is not None:
        fields.append("user_name = ?")
        values.append(username)
    if email is not None:
        fields.append("email = ?")
        values.append(email)
    if password_hash is not None:
        fields.append("password = ?")
        values.append(password_hash)
    
    if fields:
        values.append(user_id)
        sql = f"UPDATE users SET {', '.join(fields)} WHERE id = ?"
        cursor.execute(sql, values)
        conn.commit()
    
    conn.close()

if __name__ == "__main__":
    # Example usage
    update_user(user_id='bdf40350-ea99-42f7-a96e-2cdf30e7bfae', username='test', email='test@test.test', password_hash='$2a$10$oWWB27x9CkbjQCEw2fXfUOTzq5hJ83AivolrVaBQQWmUiuiqGH2M.')

