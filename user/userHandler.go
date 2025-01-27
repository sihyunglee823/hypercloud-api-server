package user

import (
	"net/http"

	"github.com/tmax-cloud/hypercloud-api-server/util"
	k8sApiCaller "github.com/tmax-cloud/hypercloud-api-server/util/caller"

	rbacApi "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"
)

func Post(res http.ResponseWriter, req *http.Request) {
	klog.Infoln("**** POST /user")
	queryParams := req.URL.Query()
	userId := queryParams.Get("userId")

	if userId == "" {
		out := "userId is Missing"
		util.SetResponse(res, out, nil, http.StatusBadRequest)
		klog.Errorf("userId is Missing")
		return
	}

	klog.Infoln("userId is : " + userId)

	//Call CreateClusterRoleBinding for New User
	clusterRoleBinding := rbacApi.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: userId,
		},
		RoleRef: rbacApi.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "clusterrole-new-user",
		},
		Subjects: []rbacApi.Subject{
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "User",
				Name:     userId,
			},
		},
	}
	k8sApiCaller.CreateClusterRoleBinding(&clusterRoleBinding)
	out := "Create ClusterRoleBinding for New User Success"
	util.SetResponse(res, out, nil, http.StatusOK)
}

func Delete(res http.ResponseWriter, req *http.Request) {
	klog.Infoln("**** DELETE /user")
	queryParams := req.URL.Query()
	userId := queryParams.Get(util.QUERY_PARAMETER_USER_ID)

	if userId == "" {
		out := "userId is Missing"
		util.SetResponse(res, out, nil, http.StatusBadRequest)
		klog.Errorf("userId is Missing")
		return
	}

	klog.Infoln("userId is : " + userId)

	//Call Delete resource function for New User
	k8sApiCaller.DeleteClusterRoleBinding(userId)
	k8sApiCaller.DeleteRQCWithUser(userId)
	k8sApiCaller.DeleteNSCWithUser(userId)
	k8sApiCaller.DeleteRBCWithUser(userId)
	k8sApiCaller.DeleteCRBWithUser(userId)
	k8sApiCaller.DeleteRBWithUser(userId)
	out := "Successfully delete related resources with " + userId
	klog.Infoln(out)
	util.SetResponse(res, out, nil, http.StatusOK)
}

func Options(res http.ResponseWriter, req *http.Request) {
	klog.Infoln("**** OPTIONS/user")
	out := "**** OPTIONS/user"
	util.SetResponse(res, out, nil, http.StatusOK)
}
