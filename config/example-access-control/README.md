## Example Access Control Config

This is a simple example for defining simple access control to your MCP tools exposed by the gateway.


### Modify the acl

You can change the `config.json` in this directory. Note the id specified is currently defined as the server prefix. 

Example adding another group: here we have added the developers group to the test_ mcp server.


```json
{
    "acls": [
        {
            "id": "test_",
            "access": {
                "accounting": [
                    "test_headers",
                    "test_echo"
                ],
                "developers":[
                    "test_hello_world"
                ]
            }
        },
        {
            "id": "test2_",
            "access": {
                "accounting": [
                    "test2_headers",
                    "test2_echo"
                ]
            }
        }
    ]
}
```

Having saves this change you can apply it to your local environment with the following command executed from within this directory:

```sh

kubectl apply -k . 

```
