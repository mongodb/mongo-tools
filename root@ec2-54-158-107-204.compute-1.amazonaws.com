projects:
- name: mongodb-mongo-master
  default: true
  alias: required
  tasks:
  - all
user: "varsha.subrahmanyam"
api_key: "b2b7b7f3f47377a184e1a1a4d8a13ceb"
api_server_host: "https://evergreen.mongodb.com/api"
ui_server_host: "https://evergreen.mongodb.com"
